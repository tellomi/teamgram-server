package ttlmedia

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// Copy is one user-box row the one-off command may rewrite.
// MessageData is the stored JSON. The command writes it to the backup file
// and does not print it.
type Copy struct {
	Table       string
	UserID      int64
	BoxID       int32
	DialogID    int64
	MediaUnread bool
	MessageData string
	TTL         int32
	JSONUnread  bool
	VoiceRound  bool
	Kind        string
	MediaID     int64
}

// Dialog is every live copy of one ttl message.
type Dialog struct {
	DialogID int64
	TTL      int32
	Class    Class
	Kind     string
	MediaID  int64
	Copies   []Copy
}

// Scan finds ttl media in the user message tables and classifies each dialog.
// Unopened messages are classified and then left alone.
func (s *Store) Scan(ctx context.Context, now int32) ([]Dialog, error) {
	if s == nil {
		return nil, nil
	}
	opens, err := s.openRows(ctx)
	if err != nil {
		return nil, err
	}
	tables, err := s.messageTables(ctx)
	if err != nil {
		return nil, err
	}
	byDialog := map[int64]*Dialog{}
	for _, table := range tables {
		if err = s.scanTable(ctx, table, byDialog); err != nil {
			return nil, err
		}
	}
	out := make([]Dialog, 0, len(byDialog))
	for _, d := range byDialog {
		d.Class = dialogClass(d, opens[d.DialogID], now)
		out = append(out, *d)
	}
	return out, nil
}

func dialogClass(d *Dialog, open *OpenRow, now int32) Class {
	best := ClassSkipNoSignal
	for _, c := range d.Copies {
		switch Classify(c.TTL, c.MediaUnread, c.JSONUnread, c.VoiceRound, open, now) {
		case ClassExpire:
			return ClassExpire
		case ClassSkipUnopened:
			best = ClassSkipUnopened
		case ClassSkipNotDue:
			if best != ClassSkipUnopened {
				best = ClassSkipNotDue
			}
		}
	}
	return best
}

func (s *Store) openRows(ctx context.Context) (map[int64]*OpenRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT dialog_message_id, opened_at, stripped FROM message_ttl_open`)
	if err != nil {
		if missing(err) {
			return map[int64]*OpenRow{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*OpenRow{}
	for rows.Next() {
		var id int64
		var opened int32
		var stripped int
		if err = rows.Scan(&id, &opened, &stripped); err != nil {
			return nil, err
		}
		out[id] = &OpenRow{OpenedAt: opened, Stripped: stripped != 0}
	}
	return out, rows.Err()
}

func (s *Store) scanTable(ctx context.Context, table string, into map[int64]*Dialog) error {
	qtable := quote(table)
	if qtable == "" {
		return nil
	}
	var userID int64
	var boxID int32
	for {
		rows, err := s.db.QueryContext(ctx, `
SELECT user_id, user_message_box_id, dialog_message_id, media_unread, CAST(message_data AS CHAR)
FROM `+qtable+`
WHERE deleted = 0
  AND (user_id > ? OR (user_id = ? AND user_message_box_id > ?))
  AND JSON_EXTRACT(message_data, '$.media.ttl_seconds.value') > 0
ORDER BY user_id, user_message_box_id
LIMIT 400`, userID, userID, boxID)
		if err != nil {
			if missing(err) {
				return nil
			}
			return err
		}
		n := 0
		for rows.Next() {
			var c Copy
			var unread int
			c.Table = table
			if err = rows.Scan(&c.UserID, &c.BoxID, &c.DialogID, &unread, &c.MessageData); err != nil {
				rows.Close()
				return err
			}
			n++
			userID, boxID = c.UserID, c.BoxID
			c.MediaUnread = unread != 0
			fillCopy(&c)
			if c.TTL <= 0 {
				continue
			}
			d := into[c.DialogID]
			if d == nil {
				d = &Dialog{DialogID: c.DialogID, TTL: c.TTL, Kind: c.Kind, MediaID: c.MediaID}
				into[c.DialogID] = d
			}
			if d.MediaID == 0 && c.MediaID != 0 {
				d.Kind, d.MediaID = c.Kind, c.MediaID
			}
			if c.TTL != 0 {
				d.TTL = c.TTL
			}
			d.Copies = append(d.Copies, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if n < 400 {
			return nil
		}
	}
}

func fillCopy(c *Copy) {
	var msg mtproto.Message
	if jsonx.UnmarshalFromString(c.MessageData, &msg) != nil {
		return
	}
	c.TTL = mtproto.TTLSeconds(&msg)
	c.JSONUnread = msg.GetMediaUnread()
	c.VoiceRound = mtproto.IsVoiceMessage(&msg) || mtproto.IsRoundVideoMessage(&msg)
	c.Kind, c.MediaID = mediaOf(&msg)
}

// Remember records a legacy dialog that is about to be expired so a retry
// after a crash still knows the media id and does not treat it as unopened.
func (s *Store) Remember(ctx context.Context, d Dialog, now int32) error {
	if s == nil || d.DialogID == 0 {
		return nil
	}
	return s.record(ctx, d.DialogID, d.TTL, now, now, d.Kind, d.MediaID)
}
