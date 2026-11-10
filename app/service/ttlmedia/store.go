// Package ttlmedia stores when a recipient opens view-once or timed media
// and rewrites both copies to the expired form. Object deletion is not done
// here. After the message JSON no longer names the file, mediapurge.Note
// drops the ref and enqueues media_purge_queue. That worker (PR 284) deletes
// the R2 object after its grace period, and only when nothing else still
// references it. upload.getFile is refused immediately by documents/photos
// expired_at, including during that grace window.
package ttlmedia

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
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
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err = s.ensure(ctx); err != nil {
		_ = db.Close()
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
// the file. A second call is a no-op aside from the same ref drop.
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
	seen := map[mtproto.RemovedMedia]bool{}
	var removed []mtproto.RemovedMedia
	for i := range boxes {
		raw, gone, err := rewrite(boxes[i].MessageData)
		if err != nil {
			return err
		}
		for _, m := range gone {
			if !seen[m] {
				seen[m] = true
				removed = append(removed, m)
			}
		}
		if raw != boxes[i].MessageData {
			if err = s.save(ctx, boxes[i], raw); err != nil {
				return err
			}
			boxes[i].MessageData = raw
		}
	}
	if len(boxes) == 0 {
		// Nothing left to rewrite. Do not mark the file expired: a missed
		// shard would still be showing it.
		_, err = s.db.ExecContext(ctx, `UPDATE message_ttl_open SET stripped = 1 WHERE dialog_message_id = ?`, dialogMessageID)
		return err
	}
	if len(removed) == 0 && fallback.ID != 0 && !seen[fallback] {
		removed = append(removed, fallback)
	}
	purge := mediapurge.Default()
	for _, b := range boxes {
		if purge == nil {
			return fmt.Errorf("ttl media: media purge store is not bound")
		}
		if err = purge.Note(ctx, mediapurge.Box{
			Kind:  mediapurge.BoxUser,
			Owner: b.UserID,
			ID:    b.BoxID,
			Media: mediapurge.FromJSON(b.MessageData),
		}); err != nil {
			return err
		}
	}
	now := time.Now().Unix()
	for _, m := range removed {
		if err = s.refuseDownload(ctx, m.Kind, m.ID, now); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE message_ttl_open SET stripped = 1 WHERE dialog_message_id = ?`, dialogMessageID)
	return err
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

func (s *Store) save(ctx context.Context, b boxRow, raw string) error {
	qtable := quote(b.Table)
	if qtable == "" {
		return nil
	}
	// Leave media_unread for the inbox consumer. It sends
	// updateReadMessagesContents only while the column is still set, and this
	// rewrite often commits before that consumer runs.
	_, err := s.db.ExecContext(ctx, `
UPDATE `+qtable+` SET message_data = ?
WHERE user_id = ? AND user_message_box_id = ? AND deleted = 0`, raw, b.UserID, b.BoxID)
	return err
}

// refuseDownload sets expired_at when mediapurge no longer sees a holder.
// The object itself stays until the purge worker's grace elapses. getFile
// already returns FILE_ID_INVALID for expired_at != 0, so the file is
// refused during that window. A remaining holder keeps the row downloadable.
func (s *Store) refuseDownload(ctx context.Context, kind string, id, now int64) error {
	if id == 0 || (kind != mtproto.MediaKindPhoto && kind != mtproto.MediaKindDocument) {
		return nil
	}
	store := mediapurge.Default()
	if store == nil {
		return nil
	}
	hold, err := store.QuickHold(ctx, mediapurge.Media{Kind: kind, ID: id})
	if err != nil {
		return err
	}
	if hold != mediapurge.HoldNone {
		return nil
	}
	table, col := "documents", "document_id"
	if kind == mtproto.MediaKindPhoto {
		table, col = "photos", "photo_id"
	}
	_, err = s.db.ExecContext(ctx, `UPDATE `+table+` SET expired_at = ? WHERE `+col+` = ? AND expired_at = 0`, now, id)
	if missing(err) || missingColumn(err) {
		return nil
	}
	return err
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
