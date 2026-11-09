package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/historyttl"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ExpireDue permanently deletes channel and supergroup messages whose
// ttl_period has elapsed and pushes updateDeleteChannelMessages. A second
// call finds the rows already deleted and pushes nothing.
func (s *Service) ExpireDue(ctx context.Context, now int32, limit int) (int, error) {
	if s == nil || s.store == nil || limit <= 0 {
		return 0, nil
	}
	if !s.ttlAlreadyBackfilled() {
		var err error
		switch st := s.store.(type) {
		case *mysqlStore:
			err = s.backfillMySQL(ctx, st)
		case *memoryStore:
			err = s.backfillMemory(ctx, st)
		}
		if err == nil {
			s.markTTLBackfilled()
		}
	}
	switch st := s.store.(type) {
	case *mysqlStore:
		return s.expireMySQL(ctx, st, now, limit)
	case *memoryStore:
		return s.expireMemory(ctx, st, now, limit)
	default:
		return 0, nil
	}
}

func (s *Service) ttlAlreadyBackfilled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ttlBackfill
}

func (s *Service) markTTLBackfilled() {
	s.mu.Lock()
	s.ttlBackfill = true
	s.mu.Unlock()
}

func (s *Service) backfillMySQL(ctx context.Context, st *mysqlStore) error {
	rows, err := st.db.QueryContext(ctx, `SELECT id FROM channels WHERE deleted=0 AND ttl_period>0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.backfillChannel(ctx, id, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) backfillMemory(ctx context.Context, st *memoryStore) error {
	st.mu.Lock()
	var ids []int64
	for id, ch := range st.channels {
		if ch != nil && !ch.Deleted && ch.TtlPeriod > 0 {
			ids = append(ids, id)
		}
	}
	st.mu.Unlock()
	for _, id := range ids {
		if err := s.backfillChannel(ctx, id, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) backfillChannel(ctx context.Context, channelID int64, mysql bool) error {
	msgs, err := s.store.ListMessages(ctx, channelID)
	if err != nil {
		return err
	}
	var actions []historyttl.Action
	bodies := map[int32]*mtproto.Message{}
	for _, m := range msgs {
		if m == nil || m.Deleted {
			continue
		}
		body, err := unmarshalMessage(m.Data)
		if err != nil || body == nil {
			continue
		}
		bodies[m.ID] = body
		if body.GetAction().GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL {
			continue
		}
		date := body.GetDate()
		if date == 0 {
			date = m.Date
		}
		actions = append(actions, historyttl.Action{Date: date, Period: body.GetAction().GetPeriod()})
	}
	if len(actions) == 0 {
		return nil
	}
	for _, m := range msgs {
		if m == nil || m.Deleted {
			continue
		}
		body := bodies[m.ID]
		if body == nil || body.GetTtlPeriod().GetValue() > 0 {
			continue
		}
		period := historyttl.PeriodAt(actions, m.Date)
		if period <= 0 {
			continue
		}
		body.TtlPeriod = &wrapperspb.Int32Value{Value: period}
		raw, err := proto.Marshal(body)
		if err != nil {
			return err
		}
		if err := s.store.ReplaceMessageBody(ctx, channelID, m.ID, raw); err != nil {
			return err
		}
		if mysql {
			if ms, ok := s.store.(*mysqlStore); ok {
				if _, err := ms.db.ExecContext(ctx, `UPDATE channel_messages SET ttl_period=? WHERE channel_id=? AND message_id=? AND ttl_period=0`, period, channelID, m.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Service) expireMySQL(ctx context.Context, st *mysqlStore, now int32, limit int) (int, error) {
	rows, err := st.db.QueryContext(ctx, `
SELECT channel_id, message_id FROM channel_messages
WHERE deleted=0 AND ttl_period>0 AND date2>0 AND date2 + ttl_period <= ?
ORDER BY date2 LIMIT ?`, now, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	grouped := map[int64][]int32{}
	var order []int64
	for rows.Next() {
		var channelID int64
		var id int32
		if err := rows.Scan(&channelID, &id); err != nil {
			return 0, err
		}
		if _, ok := grouped[channelID]; !ok {
			order = append(order, channelID)
		}
		grouped[channelID] = append(grouped[channelID], id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int
	for _, channelID := range order {
		deleted, err := s.finishExpire(ctx, channelID, grouped[channelID])
		if err != nil {
			return n, err
		}
		n += deleted
	}
	return n, nil
}

func (s *Service) expireMemory(ctx context.Context, st *memoryStore, now int32, limit int) (int, error) {
	st.mu.Lock()
	grouped := map[int64][]int32{}
	var order []int64
	for channelID, msgs := range st.messages {
		for _, m := range msgs {
			if m == nil || m.Deleted {
				continue
			}
			body, err := unmarshalMessage(m.Data)
			if err != nil || body == nil {
				continue
			}
			if !historyttl.Expired(m.Date, body.GetTtlPeriod().GetValue(), now) {
				continue
			}
			if _, ok := grouped[channelID]; !ok {
				order = append(order, channelID)
			}
			grouped[channelID] = append(grouped[channelID], m.ID)
			if limit > 0 && countIDs(grouped) >= limit {
				break
			}
		}
	}
	st.mu.Unlock()
	var n int
	for _, channelID := range order {
		deleted, err := s.finishExpire(ctx, channelID, grouped[channelID])
		if err != nil {
			return n, err
		}
		n += deleted
	}
	return n, nil
}

func countIDs(grouped map[int64][]int32) int {
	n := 0
	for _, ids := range grouped {
		n += len(ids)
	}
	return n
}

func (s *Service) finishExpire(ctx context.Context, channelID int64, ids []int32) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	pts, count, deleted, err := s.store.CommitDelete(ctx, channelID, ids)
	if err != nil || count == 0 {
		return 0, err
	}
	msgs, err := s.store.ListMessages(ctx, channelID)
	if err != nil {
		return int(count), err
	}
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return int(count), err
	}
	var top int32
	for _, m := range msgs {
		if m != nil && !m.Deleted && m.ID > top {
			top = m.ID
		}
	}
	if ch.TopMessage != top {
		ch.TopMessage = top
		if err := s.store.SaveChannel(ctx, ch); err != nil {
			return int(count), err
		}
	}
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return int(count), err
	}
	for _, p := range parts {
		if p.State != StateNormal {
			continue
		}
		st, err := s.store.GetRead(ctx, channelID, p.UserID)
		if err != nil || st == nil {
			continue
		}
		var unread int32
		for _, m := range msgs {
			if m != nil && !m.Deleted && m.ID > st.ReadInboxMaxID {
				unread++
			}
		}
		if st.UnreadCount != unread {
			st.UnreadCount = unread
			_ = s.store.SaveRead(ctx, st)
		}
	}
	s.emit(ctx, 0, 0, channelID, func(viewer int64) *mtproto.Updates {
		return s.updatesWith(ctx, viewer, ch, deleteUpdate(channelID, deleted, pts, count))
	})
	return int(count), nil
}
