// Package ttlmedia stores when a recipient opens view-once or timed media
// and rewrites both copies to the expired form. Object deletion is not done
// here. The same transaction that rewrites message_data first sets
// documents/photos.expired_at (when nothing else still references the file)
// and drops message_media_refs. upload.getFile reads expired_at before it
// touches the dfs chunk cache, so a history row that already shows the
// expired form cannot be downloaded. The purge worker (PR 284) deletes the
// R2 object after its grace period, and only when nothing else still
// references it.
package ttlmedia

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/retention"
	"github.com/teamgram/teamgram-server/app/service/mediapurge"
	"github.com/zeromicro/go-zero/core/jsonx"
)

const workerInterval = time.Second

var tableName = regexp.MustCompile(`^messages(_[0-9]+)?$`)

// Store is the MySQL side of message_ttl_open plus the message rewrite.
type Store struct {
	db *sql.DB

	tablesMu sync.Mutex
	tables   []string

	// holdOutside, when set, replaces the mediapurge holder check. Production
	// leaves it nil. Tests use it to separate "who else holds the file" from
	// the order of the writes.
	holdOutside func(ctx context.Context, kind string, id int64, own []boxRow) (bool, error)
	// forget, when set, is called in addition to retention.Forget. Tests use
	// it to observe that the getFile cache drop happens before the history
	// rewrite.
	forget func(kind string, id int64)
}

var (
	bindMu sync.Mutex
	bound  *Store
)

// Open connects, creates message_ttl_open, and installs the process-wide
// store. An empty dsn leaves the store unset. mediapurge.Bind is called on
// the same pool so Note sees the refs this process writes.
func Open(dsn string) (*Store, error) {
	dsn = trim(dsn)
	if dsn == "" {
		return nil, nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = mysqlpool.Discard(db)
		return nil, err
	}
	s := &Store{db: db}
	if err = s.ensure(ctx); err != nil {
		_ = mysqlpool.Discard(db)
		return nil, err
	}
	mediapurge.Bind(db)
	bindMu.Lock()
	bound = s
	bindMu.Unlock()
	return s, nil
}

// Default is the store installed by Open. Nil means ttl tracking is off.
func Default() *Store {
	bindMu.Lock()
	defer bindMu.Unlock()
	return bound
}

// Start runs the due-row worker until the process exits. A nil store is ignored.
func Start(s *Store) {
	if s == nil {
		return
	}
	go s.Loop(context.Background())
}

func (s *Store) ensure(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schemaSQL)
	return err
}

// Loop rewrites messages whose open countdown has elapsed.
func (s *Store) Loop(ctx context.Context) {
	if s == nil {
		return
	}
	tick := time.NewTicker(workerInterval)
	defer tick.Stop()
	s.pass(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.pass(ctx)
		}
	}
}

func (s *Store) pass(ctx context.Context) {
	n, err := s.ExpireDue(ctx, int32(time.Now().Unix()), 100)
	if err != nil {
		log.Printf("ttl media: expire due: %v (stripped=%d)", err, n)
	} else if n > 0 {
		log.Printf("ttl media: stripped=%d", n)
	}
}

// Opened records that the recipient read a ttl message at now.
// due is true for view-once (and for a timed message whose countdown has
// already elapsed). The caller must notify the sender before Strip: Strip
// clears media_unread, and the inbox push is skipped once that flag is gone.
func (s *Store) Opened(ctx context.Context, dialogMessageID int64, msg *mtproto.Message, now int32) (bool, error) {
	if s == nil || dialogMessageID == 0 {
		return false, nil
	}
	ttl := mtproto.TTLSeconds(msg)
	if ttl == 0 {
		return false, nil
	}
	kind, id := mediaOf(msg)
	expireAt := now + ttl
	if mtproto.IsViewOnce(ttl) {
		expireAt = now
	}
	if err := s.record(ctx, dialogMessageID, ttl, now, expireAt, kind, id); err != nil {
		return false, err
	}
	return mtproto.TTLDue(ttl, now, now), nil
}

func mediaOf(msg *mtproto.Message) (string, int64) {
	if msg == nil || msg.GetMedia() == nil {
		return "", 0
	}
	if p := msg.GetMedia().GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 && p.GetPredicateName() != mtproto.Predicate_photoEmpty {
		return mtproto.MediaKindPhoto, p.GetId()
	}
	if d := msg.GetMedia().GetDocument(); d != nil && d.GetId() != 0 && d.GetPredicateName() != mtproto.Predicate_documentEmpty {
		return mtproto.MediaKindDocument, d.GetId()
	}
	return "", 0
}

func (s *Store) record(ctx context.Context, dialogID int64, ttl, openedAt, expireAt int32, kind string, id int64) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO message_ttl_open (dialog_message_id, ttl_seconds, opened_at, expire_at, media_kind, media_id, stripped)
VALUES (?,?,?,?,?,?,0)
ON DUPLICATE KEY UPDATE
  ttl_seconds = IF(ttl_seconds = 0, VALUES(ttl_seconds), ttl_seconds),
  opened_at = IF(opened_at = 0, VALUES(opened_at), opened_at),
  expire_at = IF(expire_at = 0, VALUES(expire_at), expire_at),
  media_kind = IF(media_kind = '', VALUES(media_kind), media_kind),
  media_id = IF(media_id = 0, VALUES(media_id), media_id)`,
		dialogID, ttl, openedAt, expireAt, kind, id)
	return err
}

// ExpireDue rewrites every recorded dialog whose countdown has elapsed.
func (s *Store) ExpireDue(ctx context.Context, now int32, limit int) (int, error) {
	if s == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT dialog_message_id FROM message_ttl_open
WHERE stripped = 0 AND expire_at > 0 AND expire_at <= ?
ORDER BY expire_at LIMIT ?`, now, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err = s.Strip(ctx, id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Strip rewrites every live user-box copy of dialogMessageID to the expired
// form, drops mediapurge refs, and refuses getFile when nothing else holds
// the file. View-once and timed media share this path. View-once is still
// invoked from readMessageContents before that RPC returns.
//
// expired_at is updated before any message_data write, inside the same
// transaction. A crash rolls the whole unit back, so a retry cannot observe
// an expired history row whose file is still downloadable. A second call
// finishes anything the previous attempt left (history already empty,
// expired_at still 0) and is otherwise a no-op aside from the same ref drop.
func (s *Store) Strip(ctx context.Context, dialogMessageID int64) error {
	if s == nil || dialogMessageID == 0 {
		return nil
	}
	fallback, err := s.savedMedia(ctx, dialogMessageID)
	if err != nil {
		return err
	}
	tables, err := s.messageTables(ctx)
	if err != nil {
		return err
	}
	var boxes []boxRow
	for _, table := range tables {
		part, err := s.boxes(ctx, table, dialogMessageID)
		if err != nil {
			return err
		}
		boxes = append(boxes, part...)
	}
	if len(boxes) == 0 {
		// Nothing left to rewrite. Do not mark the file expired: a missed
		// shard would still be showing it.
		_, err = s.db.ExecContext(ctx, `UPDATE message_ttl_open SET stripped = 1 WHERE dialog_message_id = ?`, dialogMessageID)
		return err
	}
	plans, removed, err := planStrip(boxes, fallback)
	if err != nil {
		return err
	}
	doomed, err := s.chooseExpired(ctx, removed, boxes)
	if err != nil {
		return err
	}
	return s.commitStrip(ctx, dialogMessageID, plans, doomed)
}

type stripPlan struct {
	box     boxRow
	raw     string
	changed bool
}

func planStrip(boxes []boxRow, fallback mtproto.RemovedMedia) ([]stripPlan, []mtproto.RemovedMedia, error) {
	seen := map[mtproto.RemovedMedia]bool{}
	var removed []mtproto.RemovedMedia
	plans := make([]stripPlan, len(boxes))
	for i := range boxes {
		raw, gone, err := rewrite(boxes[i].MessageData)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range gone {
			if seen[m] {
				continue
			}
			seen[m] = true
			removed = append(removed, m)
		}
		plans[i] = stripPlan{box: boxes[i], raw: raw, changed: raw != boxes[i].MessageData}
	}
	if len(removed) == 0 && fallback.ID != 0 && !seen[fallback] {
		removed = append(removed, fallback)
	}
	return plans, removed, nil
}

// chooseExpired decides which ids getFile must refuse. The check runs before
// any write. Boxes in this dialog do not count: they are the copies this
// transaction is about to detach. Any other holder keeps the file.
func (s *Store) chooseExpired(ctx context.Context, removed []mtproto.RemovedMedia, boxes []boxRow) ([]mtproto.RemovedMedia, error) {
	ready := map[string]bool{}
	seen := map[mtproto.RemovedMedia]bool{}
	var doomed []mtproto.RemovedMedia
	for _, m := range removed {
		if m.ID == 0 || seen[m] {
			continue
		}
		seen[m] = true
		if m.Kind != mtproto.MediaKindPhoto && m.Kind != mtproto.MediaKindDocument {
			continue
		}
		held, err := s.othersHold(ctx, m.Kind, m.ID, boxes)
		if err != nil {
			return nil, err
		}
		if held {
			continue
		}
		ok, known := ready[m.Kind]
		if !known {
			ok, err = s.expireReady(ctx, m.Kind)
			if err != nil {
				return nil, err
			}
			ready[m.Kind] = ok
		}
		if !ok {
			continue
		}
		doomed = append(doomed, m)
	}
	return doomed, nil
}

func (s *Store) othersHold(ctx context.Context, kind string, id int64, own []boxRow) (bool, error) {
	if s.holdOutside != nil {
		return s.holdOutside(ctx, kind, id, own)
	}
	p := mediapurge.Default()
	if p == nil {
		return false, fmt.Errorf("ttl media: media purge store is not bound")
	}
	except := make([]mediapurge.ExceptBox, len(own))
	for i, b := range own {
		except[i] = mediapurge.ExceptBox{Owner: b.UserID, ID: b.BoxID}
	}
	return p.HeldOutside(ctx, mediapurge.Media{Kind: kind, ID: id}, except)
}

func (s *Store) expireReady(ctx context.Context, kind string) (bool, error) {
	table, _, ok := expireTarget(kind)
	if !ok {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'expired_at'`, table).Scan(&n)
	if missing(err) || missingColumn(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func expireTarget(kind string) (table, col string, ok bool) {
	switch kind {
	case mtproto.MediaKindDocument:
		return "documents", "document_id", true
	case mtproto.MediaKindPhoto:
		return "photos", "photo_id", true
	default:
		return "", "", false
	}
}

// commitStrip publishes expired_at before message_data. Both commit together,
// so a reader cannot see the expired history while expired_at is still 0.
func (s *Store) commitStrip(ctx context.Context, dialogID int64, plans []stripPlan, doomed []mtproto.RemovedMedia) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	doomedSet := make(map[mediapurge.Media]bool, len(doomed))
	for _, m := range doomed {
		key := mediapurge.Media{Kind: m.Kind, ID: m.ID}
		doomedSet[key] = true
		// Drop a cached "still downloadable" answer before the history row
		// changes. upload.getFile checks expired_at before the dfs chunk
		// cache, and it does not keep a negative expiry answer.
		s.forgetDownload(m.Kind, m.ID)
		if err = markExpiredTx(ctx, tx, m.Kind, m.ID, now); err != nil {
			return err
		}
	}
	for _, p := range plans {
		if !p.changed {
			continue
		}
		if err = saveTx(ctx, tx, p.box, p.raw); err != nil {
			return err
		}
	}
	dropped := map[mediapurge.Media]bool{}
	for _, p := range plans {
		gone, err := mediapurge.SyncBoxTx(ctx, tx, mediapurge.Box{
			Kind:  mediapurge.BoxUser,
			Owner: p.box.UserID,
			ID:    p.box.BoxID,
			Media: mediapurge.FromJSON(p.raw),
		})
		if err != nil {
			return err
		}
		for _, m := range gone {
			dropped[m] = true
		}
	}
	when := time.Now().Add(mediapurge.Grace())
	queue := make([]mediapurge.Media, 0, len(dropped))
	for m := range dropped {
		if doomedSet[m] {
			queue = append(queue, m)
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		if queue[i].Kind == queue[j].Kind {
			return queue[i].ID < queue[j].ID
		}
		return queue[i].Kind < queue[j].Kind
	})
	for _, m := range queue {
		if err = mediapurge.EnqueueTx(ctx, tx, m, when); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE message_ttl_open SET stripped = 1 WHERE dialog_message_id = ?`, dialogID); err != nil {
		return err
	}
	return tx.Commit()
}

func markExpiredTx(ctx context.Context, tx *sql.Tx, kind string, id, now int64) error {
	table, col, ok := expireTarget(kind)
	if !ok || id == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE `+table+` SET expired_at = ? WHERE `+col+` = ? AND expired_at = 0`, now, id)
	return err
}

func saveTx(ctx context.Context, tx *sql.Tx, b boxRow, raw string) error {
	qtable := quote(b.Table)
	if qtable == "" {
		return nil
	}
	// Leave media_unread for the inbox consumer. It sends
	// updateReadMessagesContents only while the column is still set, and this
	// rewrite often commits before that consumer runs.
	_, err := tx.ExecContext(ctx, `
UPDATE `+qtable+` SET message_data = ?
WHERE user_id = ? AND user_message_box_id = ? AND deleted = 0`, raw, b.UserID, b.BoxID)
	return err
}

// forgetDownload drops this process's cached "not expired" answer. The files
// service that serves upload.getFile does not share this process's memory;
// that check re-reads expired_at whenever the cached answer would have been
// "still live".
func (s *Store) forgetDownload(kind string, id int64) {
	if s != nil && s.forget != nil {
		s.forget(kind, id)
	}
	switch kind {
	case mtproto.MediaKindPhoto:
		retention.Forget(retention.KindPhoto, id)
	case mtproto.MediaKindDocument:
		retention.Forget(retention.KindDocument, id)
	}
}

func (s *Store) savedMedia(ctx context.Context, dialogID int64) (mtproto.RemovedMedia, error) {
	var kind string
	var id int64
	err := s.db.QueryRowContext(ctx, `
SELECT media_kind, media_id FROM message_ttl_open WHERE dialog_message_id = ?`, dialogID).Scan(&kind, &id)
	if err == sql.ErrNoRows {
		return mtproto.RemovedMedia{}, nil
	}
	if err != nil {
		return mtproto.RemovedMedia{}, err
	}
	return mtproto.RemovedMedia{Kind: kind, ID: id}, nil
}

type boxRow struct {
	Table       string
	UserID      int64
	BoxID       int32
	MessageData string
}

func (s *Store) boxes(ctx context.Context, table string, dialogID int64) ([]boxRow, error) {
	qtable := quote(table)
	if qtable == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT user_id, user_message_box_id, CAST(message_data AS CHAR)
FROM `+qtable+` WHERE dialog_message_id = ? AND deleted = 0`, dialogID)
	if err != nil {
		if missing(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []boxRow
	for rows.Next() {
		var b boxRow
		b.Table = table
		if err = rows.Scan(&b.UserID, &b.BoxID, &b.MessageData); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func rewrite(raw string) (string, []mtproto.RemovedMedia, error) {
	if raw == "" || raw == "{}" || raw == "null" {
		return raw, nil, nil
	}
	var msg mtproto.Message
	if err := jsonx.UnmarshalFromString(raw, &msg); err != nil {
		return raw, nil, err
	}
	removed := mtproto.StripTTLMedia(&msg)
	if mtproto.TTLSeconds(&msg) == 0 {
		return raw, nil, nil
	}
	buf, err := jsonx.Marshal(&msg)
	if err != nil {
		return raw, nil, err
	}
	return string(buf), removed, nil
}

func (s *Store) messageTables(ctx context.Context) ([]string, error) {
	s.tablesMu.Lock()
	defer s.tablesMu.Unlock()
	if len(s.tables) > 0 {
		return s.tables, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND (table_name = 'messages' OR table_name REGEXP '^messages_[0-9]+$')
ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		if tableName.MatchString(name) {
			tables = append(tables, name)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		tables = []string{"messages"}
	}
	s.tables = tables
	return tables, nil
}

func quote(name string) string {
	if !tableName.MatchString(name) {
		return ""
	}
	return "`" + name + "`"
}

func missing(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}

func missingColumn(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1054
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
