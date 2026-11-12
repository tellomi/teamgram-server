package mediapurge

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"

	mysqldriver "github.com/go-sql-driver/mysql"
)

var messageTableName = regexp.MustCompile(`^messages(_[0-9]+)?$`)

// Hold is why an object must stay. HoldNone means nothing in this database
// still points at the photo_id or document_id.
type Hold string

const (
	HoldNone      Hold = ""
	HoldMessage   Hold = "message"
	HoldPinned    Hold = "pinned"
	HoldChannel   Hold = "channel"
	HoldAvatar    Hold = "avatar"
	HoldStory     Hold = "story"
	HoldSticker   Hold = "sticker"
	HoldSaved     Hold = "saved"
	HoldScheduled Hold = "scheduled"
	HoldExternal  Hold = "external"
)

// ExceptBox is a user message the caller is about to detach. HeldOutside
// does not treat it as a reason to keep the file.
type ExceptBox struct {
	Owner int64
	ID    int32
}

// HeldOutside reports whether something other than own still references m.
// own are the user boxes this operation is about to stop pointing at the
// file. A query error is returned so the caller keeps the object.
func (s *Store) HeldOutside(ctx context.Context, m Media, own []ExceptBox) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("media purge store is not bound")
	}
	if m.ID == 0 {
		return false, nil
	}
	h, err := s.userMessageHoldExcept(ctx, m, own)
	if err != nil || h != HoldNone {
		return h != HoldNone, err
	}
	h, err = s.refHoldExcept(ctx, m, own)
	if err != nil || h != HoldNone {
		return h != HoldNone, err
	}
	h, err = s.pointHold(ctx, m)
	return h != HoldNone, err
}

// QuickHold is the check a delete RPC runs. It uses the message functional
// indexes, the ref table, and the small point-lookup tables. Channel,
// story, and scheduled scans happen in the worker so the RPC stays short.
func (s *Store) QuickHold(ctx context.Context, m Media) (Hold, error) {
	if s == nil || s.db == nil || m.ID == 0 {
		return HoldNone, nil
	}
	if h, err := s.userMessageHold(ctx, m); err != nil || h != HoldNone {
		return h, err
	}
	if h, err := s.refHold(ctx, m); err != nil || h != HoldNone {
		return h, err
	}
	return s.pointHold(ctx, m)
}

// FullHold is QuickHold plus channel messages, stories, and scheduled
// messages. A scan error is returned so the caller keeps the object.
func (s *Store) FullHold(ctx context.Context, m Media, cache *holdCache) (Hold, error) {
	if h, err := s.QuickHold(ctx, m); err != nil || h != HoldNone {
		return h, err
	}
	if cache == nil {
		cache = &holdCache{}
	}
	if err := cache.load(ctx, s); err != nil {
		return HoldNone, err
	}
	if _, ok := cache.channel[m]; ok {
		return HoldChannel, nil
	}
	if _, ok := cache.story[m]; ok {
		return HoldStory, nil
	}
	if _, ok := cache.scheduled[m]; ok {
		return HoldScheduled, nil
	}
	return HoldNone, nil
}

func (s *Store) userMessageHold(ctx context.Context, m Media) (Hold, error) {
	expr := exprDocumentID
	if m.Kind == KindPhoto {
		expr = exprPhotoID
	}
	tables, err := s.messageTables(ctx)
	if err != nil {
		return HoldNone, err
	}
	for _, table := range tables {
		var pinned int
		q := `SELECT pinned FROM ` + table + ` WHERE deleted=0 AND ` + expr + `=? LIMIT 1`
		err = s.db.QueryRowContext(ctx, q, m.ID).Scan(&pinned)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			if missing(err) {
				continue
			}
			return HoldNone, err
		}
		if pinned != 0 {
			return HoldPinned, nil
		}
		return HoldMessage, nil
	}
	return HoldNone, nil
}

// userMessageHoldExcept is userMessageHold ignoring the boxes in own. The
// exclusion is in the SQL so a LIMIT 1 cannot stop on the copy being stripped
// and miss a different message that still shows the file.
func (s *Store) userMessageHoldExcept(ctx context.Context, m Media, own []ExceptBox) (Hold, error) {
	expr := exprDocumentID
	if m.Kind == KindPhoto {
		expr = exprPhotoID
	}
	tables, err := s.messageTables(ctx)
	if err != nil {
		return HoldNone, err
	}
	for _, table := range tables {
		q := `SELECT pinned FROM ` + table + ` WHERE deleted=0 AND ` + expr + `=?`
		args := []any{m.ID}
		if len(own) > 0 {
			parts := make([]string, len(own))
			for i, b := range own {
				parts[i] = "(user_id=? AND user_message_box_id=?)"
				args = append(args, b.Owner, b.ID)
			}
			q += " AND NOT (" + strings.Join(parts, " OR ") + ")"
		}
		q += " LIMIT 1"
		var pinned int
		err = s.db.QueryRowContext(ctx, q, args...).Scan(&pinned)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			if missing(err) {
				continue
			}
			return HoldNone, err
		}
		if pinned != 0 {
			return HoldPinned, nil
		}
		return HoldMessage, nil
	}
	return HoldNone, nil
}

func (s *Store) refHold(ctx context.Context, m Media) (Hold, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT box_kind, owner_id, box_id FROM message_media_refs
WHERE media_kind=? AND media_id=? LIMIT 32`, m.Kind, m.ID)
	if missing(err) {
		return HoldNone, nil
	}
	if err != nil {
		return HoldNone, err
	}
	defer rows.Close()
	tables, err := s.messageTables(ctx)
	if err != nil {
		return HoldNone, err
	}
	for rows.Next() {
		var kind int
		var owner int64
		var boxID int32
		if err = rows.Scan(&kind, &owner, &boxID); err != nil {
			return HoldNone, err
		}
		switch kind {
		case BoxChannel:
			ok, err := s.exists(ctx, `SELECT 1 FROM channel_messages WHERE channel_id=? AND message_id=? AND deleted=0 LIMIT 1`, owner, boxID)
			if err != nil {
				return HoldNone, err
			}
			if ok {
				return HoldChannel, nil
			}
		case BoxScheduled:
			return HoldScheduled, nil
		default:
			ok, err := s.userBoxLive(ctx, tables, owner, boxID)
			if err != nil {
				return HoldNone, err
			}
			if ok {
				return HoldMessage, nil
			}
		}
	}
	return HoldNone, rows.Err()
}

// refHoldExcept ignores user-box refs in own. Channel and scheduled refs
// still count. More than refExceptLimit rows is treated as a hold so a
// truncated scan cannot expire a widely shared file.
const refExceptLimit = 64

func (s *Store) refHoldExcept(ctx context.Context, m Media, own []ExceptBox) (Hold, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT box_kind, owner_id, box_id FROM message_media_refs
WHERE media_kind=? AND media_id=? LIMIT `+strconv.Itoa(refExceptLimit), m.Kind, m.ID)
	if missing(err) {
		return HoldNone, nil
	}
	if err != nil {
		return HoldNone, err
	}
	defer rows.Close()
	tables, err := s.messageTables(ctx)
	if err != nil {
		return HoldNone, err
	}
	n := 0
	for rows.Next() {
		n++
		var kind int
		var owner int64
		var boxID int32
		if err = rows.Scan(&kind, &owner, &boxID); err != nil {
			return HoldNone, err
		}
		if kind != BoxChannel && kind != BoxScheduled && boxExcluded(own, owner, boxID) {
			continue
		}
		switch kind {
		case BoxChannel:
			ok, err := s.exists(ctx, `SELECT 1 FROM channel_messages WHERE channel_id=? AND message_id=? AND deleted=0 LIMIT 1`, owner, boxID)
			if err != nil {
				return HoldNone, err
			}
			if ok {
				return HoldChannel, nil
			}
		case BoxScheduled:
			return HoldScheduled, nil
		default:
			ok, err := s.userBoxLive(ctx, tables, owner, boxID)
			if err != nil {
				return HoldNone, err
			}
			if ok {
				return HoldMessage, nil
			}
		}
	}
	if err = rows.Err(); err != nil {
		return HoldNone, err
	}
	if n == refExceptLimit {
		return HoldMessage, nil
	}
	return HoldNone, nil
}

func boxExcluded(own []ExceptBox, owner int64, id int32) bool {
	for _, b := range own {
		if b.Owner == owner && b.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) userBoxLive(ctx context.Context, tables []string, owner int64, boxID int32) (bool, error) {
	for _, table := range tables {
		ok, err := s.exists(ctx, `SELECT 1 FROM `+table+` WHERE user_id=? AND user_message_box_id=? AND deleted=0 LIMIT 1`, owner, boxID)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (s *Store) pointHold(ctx context.Context, m Media) (Hold, error) {
	if m.Kind == KindPhoto {
		checks := []struct {
			hold Hold
			q    string
		}{
			{HoldAvatar, `SELECT 1 FROM user_profile_photos WHERE photo_id=? AND deleted=0 LIMIT 1`},
			{HoldAvatar, `SELECT 1 FROM users WHERE photo_id=? AND deleted=0 LIMIT 1`},
			{HoldAvatar, `SELECT 1 FROM chats WHERE photo_id=? LIMIT 1`},
			{HoldAvatar, `SELECT 1 FROM channels WHERE photo_id=? AND deleted=0 LIMIT 1`},
			{HoldExternal, `SELECT 1 FROM media_external_refs WHERE media_kind=? AND media_id=? LIMIT 1`},
		}
		for _, c := range checks {
			args := []any{m.ID}
			if c.hold == HoldExternal {
				args = []any{m.Kind, m.ID}
			}
			ok, err := s.exists(ctx, c.q, args...)
			if err != nil {
				return HoldNone, err
			}
			if ok {
				return c.hold, nil
			}
		}
		return HoldNone, nil
	}
	checks := []struct {
		hold Hold
		q    string
	}{
		{HoldSticker, `SELECT 1 FROM tellomi_sticker_items WHERE document_id=? LIMIT 1`},
		{HoldSticker, `SELECT 1 FROM tellomi_sticker_recent WHERE document_id=? LIMIT 1`},
		{HoldSticker, `SELECT 1 FROM tellomi_sticker_faved WHERE document_id=? LIMIT 1`},
		{HoldSaved, `SELECT 1 FROM tellomi_saved_gifs WHERE document_id=? LIMIT 1`},
		{HoldSaved, `SELECT 1 FROM user_saved_music WHERE saved_music_id=? AND deleted=0 LIMIT 1`},
		{HoldSaved, `SELECT 1 FROM users WHERE emoji_status_document_id=? LIMIT 1`},
		{HoldSaved, `SELECT 1 FROM users WHERE saved_music_id=? LIMIT 1`},
		{HoldExternal, `SELECT 1 FROM media_external_refs WHERE media_kind=? AND media_id=? LIMIT 1`},
	}
	for _, c := range checks {
		args := []any{m.ID}
		if c.hold == HoldExternal {
			args = []any{m.Kind, m.ID}
		}
		ok, err := s.exists(ctx, c.q, args...)
		if err != nil {
			return HoldNone, err
		}
		if ok {
			return c.hold, nil
		}
	}
	return HoldNone, nil
}

func (s *Store) exists(ctx context.Context, q string, args ...any) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	if err == sql.ErrNoRows || missing(err) || missingColumn(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) messageTables(ctx context.Context) ([]string, error) {
	s.tablesMu.Lock()
	defer s.tablesMu.Unlock()
	if s.tables != nil {
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
	var out []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		if !messageTableName.MatchString(name) {
			continue
		}
		out = append(out, name)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []string{"messages"}
	}
	s.tables = out
	return out, nil
}

func missing(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}

func missingColumn(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1054
}

// holdCache is one worker run's scan of channel, story, and scheduled media.
type holdCache struct {
	once      sync.Once
	err       error
	channel   map[Media]struct{}
	story     map[Media]struct{}
	scheduled map[Media]struct{}
}

func (c *holdCache) load(ctx context.Context, s *Store) error {
	c.once.Do(func() {
		c.channel = map[Media]struct{}{}
		c.story = map[Media]struct{}{}
		c.scheduled = map[Media]struct{}{}
		c.err = s.scanChannels(ctx, c.channel)
		if c.err != nil {
			return
		}
		c.err = s.scanStories(ctx, c.story)
		if c.err != nil {
			return
		}
		c.err = s.scanScheduled(ctx, c.scheduled)
	})
	return c.err
}
