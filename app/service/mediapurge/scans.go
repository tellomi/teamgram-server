package mediapurge

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (s *Store) scanChannels(ctx context.Context, into map[Media]struct{}) error {
	var channelID int64
	var msgID int32
	for {
		rows, err := s.db.QueryContext(ctx, `
SELECT channel_id, message_id, message_data FROM channel_messages
WHERE deleted=0 AND (channel_id > ? OR (channel_id = ? AND message_id > ?))
ORDER BY channel_id, message_id LIMIT 400`, channelID, channelID, msgID)
		if missing(err) {
			return nil
		}
		if err != nil {
			return err
		}
		n := 0
		for rows.Next() {
			var id int64
			var mid int32
			var body []byte
			if err = rows.Scan(&id, &mid, &body); err != nil {
				rows.Close()
				return err
			}
			n++
			channelID, msgID = id, mid
			for _, m := range FromProto(body) {
				into[m] = struct{}{}
			}
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

func (s *Store) scanStories(ctx context.Context, into map[Media]struct{}) error {
	var ownerKind int
	var ownerID int64
	var storyID int
	for {
		rows, err := s.db.QueryContext(ctx, `
SELECT owner_kind, owner_id, story_id, body FROM stories
WHERE deleted=0 AND (owner_kind > ? OR (owner_kind = ? AND owner_id > ?) OR (owner_kind = ? AND owner_id = ? AND story_id > ?))
ORDER BY owner_kind, owner_id, story_id LIMIT 400`,
			ownerKind, ownerKind, ownerID, ownerKind, ownerID, storyID)
		if missing(err) {
			return nil
		}
		if err != nil {
			return err
		}
		n := 0
		for rows.Next() {
			var kind int
			var owner int64
			var id int
			var body []byte
			if err = rows.Scan(&kind, &owner, &id, &body); err != nil {
				rows.Close()
				return err
			}
			n++
			ownerKind, ownerID, storyID = kind, owner, id
			for _, m := range FromStory(body) {
				into[m] = struct{}{}
			}
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

type schedHead struct {
	Items []struct {
		Msg []byte `json:"msg"`
	} `json:"items"`
}

func (s *Store) scanScheduled(ctx context.Context, into map[Media]struct{}) error {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM chatbox_doc WHERE doc_key LIKE 'sched:%'`)
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		var doc schedHead
		if json.Unmarshal(raw, &doc) != nil {
			continue
		}
		for _, item := range doc.Items {
			for _, m := range FromProto(item.Msg) {
				into[m] = struct{}{}
			}
		}
	}
	return rows.Err()
}

// countLiveRefs is how many message_media_refs rows point at this id.
func (s *Store) countLiveRefs(ctx context.Context, m Media) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM message_media_refs WHERE media_kind=? AND media_id=?`, m.Kind, m.ID).Scan(&n)
	if missing(err) {
		return 0, nil
	}
	return n, err
}

func ignoreNoRows(err error) error {
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}
