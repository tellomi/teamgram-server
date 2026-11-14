package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// removedMessage is one channel message channels.deleteMessages is about
// to remove. The sender did not count it in their own unread_count.
type removedMessage struct {
	ID     int32
	Sender int64
}

// dropDeletedUnread subtracts messages other people had not read yet.
// A private revoke already rolls dialogs.unread_count back. A supergroup
// delete used to leave that counter, so getDialogs kept the old badge.
// Each member whose count changed also gets updateReadChannelInbox with
// still_unread_count, which is the value official clients paint.
func (s *Service) dropDeletedUnread(ctx context.Context, channelID int64, removed []removedMessage, pts int32) {
	if s == nil || s.store == nil || len(removed) == 0 {
		return
	}
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return
	}
	var changed []int64
	for _, p := range parts {
		if p.State != StateNormal || p.UserID == 0 {
			continue
		}
		st, err := s.store.GetRead(ctx, channelID, p.UserID)
		if err != nil || st == nil {
			continue
		}
		var n int32
		for _, m := range removed {
			if m.Sender == p.UserID || m.ID <= st.ReadInboxMaxID {
				continue
			}
			n++
		}
		if n == 0 || st.UnreadCount == 0 {
			continue
		}
		if st.UnreadCount > n {
			st.UnreadCount -= n
		} else {
			st.UnreadCount = 0
		}
		if err = s.store.SaveRead(ctx, st); err != nil {
			continue
		}
		changed = append(changed, p.UserID)
	}
	s.pushDeletedUnread(ctx, channelID, changed, pts)
}

func (s *Service) pushDeletedUnread(ctx context.Context, channelID int64, users []int64, pts int32) {
	h := s.hooks()
	if h.Push == nil || len(users) == 0 {
		return
	}
	for _, id := range users {
		st, err := s.store.GetRead(ctx, channelID, id)
		if err != nil || st == nil {
			continue
		}
		upd := mtproto.MakeTLUpdateReadChannelInbox(&mtproto.Update{
			ChannelId:        channelID,
			MaxId:            st.ReadInboxMaxID,
			StillUnreadCount: st.UnreadCount,
			Pts_INT32:        pts,
		}).To_Update()
		h.Push(ctx, id, nil, s.pack(ctx, id, channelID, upd))
	}
}

func removedStill(all []removedMessage, deleted []int32) []removedMessage {
	if len(all) == 0 || len(deleted) == 0 {
		return nil
	}
	live := make(map[int32]struct{}, len(deleted))
	for _, id := range deleted {
		live[id] = struct{}{}
	}
	out := make([]removedMessage, 0, len(deleted))
	for _, m := range all {
		if _, ok := live[m.ID]; ok {
			out = append(out, m)
		}
	}
	return out
}
