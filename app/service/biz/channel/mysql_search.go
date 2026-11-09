package channel

import (
	"context"
)

func (s *mysqlStore) SearchPosts(ctx context.Context, self int64, q PostQuery) ([]*StoredMessage, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	stmt := `
SELECT m.channel_id, m.message_id, m.sender_user_id, m.date2, m.edit_date, m.random_id, m.pts, m.deleted, m.message_data
FROM channel_messages m
JOIN channels c ON c.id = m.channel_id
WHERE m.deleted = 0
  AND c.deleted = 0 AND c.restricted = 0 AND c.banned = 0
  AND NOT EXISTS (
    SELECT 1 FROM channel_participants bp
    WHERE bp.channel_id = c.id AND bp.user_id = ? AND (bp.state = ? OR bp.role = ?)
  )
  AND (
    (c.broadcast = 1 AND c.username IS NOT NULL AND c.username <> '')
    OR c.id IN (
      SELECT channel_id FROM channel_participants
      WHERE user_id = ? AND state = ? AND role <> ?
    )
  )
  AND m.message LIKE ? ESCAPE '\\'`
	args := []any{self, StateKicked, RoleBanned, self, StateNormal, RoleBanned, likeContains(q.Needle)}
	if q.OffsetRate > 0 {
		stmt += `
  AND (
    m.date2 < ?
    OR (m.date2 = ? AND m.channel_id < ?)
    OR (m.date2 = ? AND m.channel_id = ? AND m.message_id < ?)
  )`
		args = append(args, q.OffsetRate, q.OffsetRate, q.OffsetPeer, q.OffsetRate, q.OffsetPeer, q.OffsetID)
	} else if q.OffsetID > 0 {
		stmt += ` AND m.message_id < ?`
		args = append(args, q.OffsetID)
	}
	stmt += ` ORDER BY m.date2 DESC, m.channel_id DESC, m.message_id DESC LIMIT ?`
	args = append(args, q.Limit)
	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*StoredMessage
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *mysqlStore) SearchPublic(ctx context.Context, self int64, q string, limit int32) ([]*Channel, error) {
	pattern := likeContains(q)
	rows, err := s.db.QueryContext(ctx, channelSelect+`
 WHERE deleted = 0 AND restricted = 0 AND banned = 0
  AND username IS NOT NULL AND username <> ''
  AND (title LIKE ? ESCAPE '\\' OR username LIKE ? ESCAPE '\\')
  AND NOT EXISTS (
    SELECT 1 FROM channel_participants bp
    WHERE bp.channel_id = channels.id AND bp.user_id = ? AND (bp.state = ? OR bp.role = ?)
  )
 ORDER BY participants_count DESC, id
 LIMIT ?`, pattern, pattern, self, StateKicked, RoleBanned, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannels(rows)
}
