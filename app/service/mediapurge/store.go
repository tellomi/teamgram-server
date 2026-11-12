package mediapurge

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"
)

// Store is the MySQL side of message_media_refs and media_purge_queue.
type Store struct {
	db       *sql.DB
	tablesMu sync.Mutex
	tables   []string
}

func New(db *sql.DB) *Store {
	if db == nil {
		return nil
	}
	return &Store{db: db}
}

func (s *Store) Ensure(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// SyncBoxTx replaces the refs for one box inside an open transaction. It
// does not commit and does not enqueue. dropped is media this box no longer
// references.
func SyncBoxTx(ctx context.Context, tx *sql.Tx, box Box) (dropped []Media, err error) {
	if tx == nil || box.Owner == 0 || box.ID == 0 {
		return nil, nil
	}
	prev, err := boxRefs(ctx, tx, box)
	if err != nil {
		return nil, err
	}
	if err = deleteBoxRefs(ctx, tx, box); err != nil {
		return nil, err
	}
	next := map[Media]struct{}{}
	for _, m := range box.Medias() {
		if _, ok := next[m]; ok {
			continue
		}
		next[m] = struct{}{}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO message_media_refs (box_kind, owner_id, peer_type, peer_id, box_id, media_kind, media_id)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE media_id = VALUES(media_id)`,
			box.Kind, box.Owner, box.PeerType, box.PeerID, box.ID, m.Kind, m.ID); err != nil {
			return nil, err
		}
	}
	for _, m := range prev {
		if _, ok := next[m]; ok {
			continue
		}
		dropped = append(dropped, m)
	}
	return dropped, nil
}

// EnqueueTx records one media id on an open transaction. A second enqueue
// keeps the earlier available_at so a repeat does not push the deadline out.
func EnqueueTx(ctx context.Context, tx *sql.Tx, m Media, available time.Time) error {
	if tx == nil || m.ID == 0 {
		return nil
	}
	now := time.Now().Unix()
	_, err := tx.ExecContext(ctx, `
INSERT INTO media_purge_queue (media_kind, media_id, enqueued_at, available_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE
  available_at = LEAST(available_at, VALUES(available_at))`,
		m.Kind, m.ID, now, available.Unix())
	return err
}

// Note replaces the media rows for one live box. A message with no media
// clears any previous rows for that box.
func (s *Store) Note(ctx context.Context, box Box) error {
	if s == nil || s.db == nil || box.Owner == 0 || box.ID == 0 {
		return nil
	}
	if err := s.Ensure(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	prev, err := boxRefs(ctx, tx, box)
	if err != nil {
		return err
	}
	if err = deleteBoxRefs(ctx, tx, box); err != nil {
		return err
	}
	next := map[Media]bool{}
	for _, m := range box.Medias() {
		next[m] = true
		if _, err = tx.ExecContext(ctx, `
INSERT INTO message_media_refs (box_kind, owner_id, peer_type, peer_id, box_id, media_kind, media_id)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE media_id = VALUES(media_id)`,
			box.Kind, box.Owner, box.PeerType, box.PeerID, box.ID, m.Kind, m.ID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	grace := Grace()
	now := time.Now()
	for _, m := range prev {
		if next[m] {
			continue
		}
		hold, err := s.QuickHold(ctx, m)
		if err != nil {
			return err
		}
		if hold != HoldNone {
			continue
		}
		if err = s.Enqueue(ctx, m, now.Add(grace)); err != nil {
			return err
		}
	}
	return nil
}

func boxRefs(ctx context.Context, tx *sql.Tx, box Box) ([]Media, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT media_kind, media_id FROM message_media_refs
WHERE box_kind=? AND owner_id=? AND peer_type=? AND peer_id=? AND box_id=?`,
		box.Kind, box.Owner, box.PeerType, box.PeerID, box.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Media
	for rows.Next() {
		var m Media
		if err = rows.Scan(&m.Kind, &m.ID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Release drops the refs for boxes that were just deleted and enqueues any
// media id that no longer has a quick hold. The object itself stays until
// the worker runs after the grace period and checks again.
func (s *Store) Release(ctx context.Context, boxes []Box, now time.Time) error {
	if s == nil || s.db == nil || len(boxes) == 0 {
		return nil
	}
	if err := s.Ensure(ctx); err != nil {
		return err
	}
	grace := Grace()
	seen := map[Media]bool{}
	var media []Media
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, box := range boxes {
		if box.ID == 0 {
			continue
		}
		if err = deleteBoxRefs(ctx, tx, box); err != nil {
			return err
		}
		for _, m := range box.Medias() {
			if seen[m] {
				continue
			}
			seen[m] = true
			media = append(media, m)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for _, m := range media {
		hold, err := s.QuickHold(ctx, m)
		if err != nil {
			return err
		}
		if hold != HoldNone {
			continue
		}
		if err = s.Enqueue(ctx, m, now.Add(grace)); err != nil {
			return err
		}
	}
	return nil
}

func deleteBoxRefs(ctx context.Context, tx *sql.Tx, box Box) error {
	_, err := tx.ExecContext(ctx, `
DELETE FROM message_media_refs
WHERE box_kind=? AND owner_id=? AND peer_type=? AND peer_id=? AND box_id=?`,
		box.Kind, box.Owner, box.PeerType, box.PeerID, box.ID)
	return err
}

// Enqueue records one media id. A second enqueue keeps the earlier
// available_at so a repeat delete does not push the deadline out.
func (s *Store) Enqueue(ctx context.Context, m Media, available time.Time) error {
	if s == nil || s.db == nil || m.ID == 0 {
		return nil
	}
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media_purge_queue (media_kind, media_id, enqueued_at, available_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE
  available_at = LEAST(available_at, VALUES(available_at))`,
		m.Kind, m.ID, now, available.Unix())
	return err
}

// DueItem is one queued media id whose grace has elapsed.
type DueItem struct {
	Kind        string
	ID          int64
	AvailableAt int64
	Attempts    int
}

// Due returns queued ids with available_at <= now. ignoreGrace returns the
// oldest rows regardless of available_at. The one-off command uses that
// after a backup, not the steady-state worker.
func (s *Store) Due(ctx context.Context, now time.Time, limit int, ignoreGrace bool) ([]DueItem, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT media_kind, media_id, available_at, attempts FROM media_purge_queue`
	args := []any{}
	if !ignoreGrace {
		q += ` WHERE available_at <= ?`
		args = append(args, now.Unix())
	}
	q += ` ORDER BY available_at, id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueItem
	for rows.Next() {
		var it DueItem
		if err = rows.Scan(&it.Kind, &it.ID, &it.AvailableAt, &it.Attempts); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) Ack(ctx context.Context, m Media) error {
	if s == nil || s.db == nil || m.ID == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM media_purge_queue WHERE media_kind=? AND media_id=?`, m.Kind, m.ID)
	return err
}

func (s *Store) Fail(ctx context.Context, m Media, when time.Time, reason string) error {
	if s == nil || s.db == nil || m.ID == 0 {
		return nil
	}
	if len(reason) > 240 {
		reason = reason[:240]
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE media_purge_queue
SET attempts = attempts + 1, last_error = ?, available_at = ?
WHERE media_kind=? AND media_id=?`, reason, when.Unix(), m.Kind, m.ID)
	return err
}

// NoteUser is the hook from a user message insert or edit.
func NoteUser(ctx context.Context, owner int64, boxID int32, messageData string) {
	s := Default()
	if s == nil || owner == 0 || boxID == 0 {
		return
	}
	box := Box{Kind: BoxUser, Owner: owner, ID: boxID, Media: FromJSON(messageData)}
	if err := s.Note(ctx, box); err != nil {
		logErr("note user %d box %d: %v", owner, boxID, err)
	}
}

// ReleaseKnown drops refs for boxes whose media ids are already known.
func ReleaseKnown(ctx context.Context, boxes []Box) {
	s := Default()
	if s == nil || len(boxes) == 0 {
		return
	}
	if err := s.Release(ctx, boxes, time.Now()); err != nil {
		logErr("release: %v", err)
	}
}

// ReleaseUser is the hook after a user box has been marked deleted and its
// text wiped. messageData is the body from before the wipe.
func ReleaseUser(ctx context.Context, owner int64, boxes []UserBox) {
	s := Default()
	if s == nil || owner == 0 || len(boxes) == 0 {
		return
	}
	out := make([]Box, 0, len(boxes))
	for _, b := range boxes {
		if b.ID == 0 {
			continue
		}
		out = append(out, Box{Kind: BoxUser, Owner: owner, ID: b.ID, Media: FromJSON(b.MessageData)})
	}
	if err := s.Release(ctx, out, time.Now()); err != nil {
		logErr("release user %d: %v", owner, err)
	}
}

// UserBox is the pre-wipe body of one user message.
type UserBox struct {
	ID          int32
	MessageData string
}

func logErr(format string, args ...any) {
	// log package is used so a service without logx still records the miss.
	// The format must not include message text, hashes, or credentials.
	logger.Printf("media purge: "+format, args...)
}
