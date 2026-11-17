package channel

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// removedMessage is one channel message channels.deleteMessages removed.
// The sender did not count it in their own unread_count.
type removedMessage struct {
	ID     int32
	Sender int64
}

// unreadSyncCap is the member count kept on the request goroutine.
// Above it, the indexed updates run in the background and nobody is
// enumerated for a push. getDialogs then reads the stored unread_count.
// updateDeleteChannelMessages is unchanged.
const unreadSyncCap int32 = 1000

// UnreadCut is one indexed update. Members of the channel whose
// read_inbox_max_id is strictly below Below, and whose user id is not
// Exclude, lose Delta from unread_count. Exclude is the message author:
// their own send never incremented their unread_count.
type UnreadCut struct {
	Below   int32
	Exclude int64
	Delta   int32
}

// unreadCutUpdateSQL is one statement per deleted message. The range
// read_inbox_max_id < ? uses KEY channel_read_inbox (channel_id,
// read_inbox_max_id). It does not walk members who have already read it.
const unreadCutUpdateSQL = `
UPDATE channel_read_state
SET unread_count = GREATEST(unread_count - ?, 0)
WHERE channel_id = ? AND read_inbox_max_id < ? AND user_id <> ? AND unread_count > 0`

const unreadCutSelectSQL = `
SELECT user_id, read_inbox_max_id, read_outbox_max_id, unread_count
FROM channel_read_state
WHERE channel_id = ? AND read_inbox_max_id < ? AND user_id <> ? AND unread_count > 0`

// Complexity, with D deleted messages, M members, and K members whose
// read_inbox_max_id is still below a cut:
//
//   - D UPDATE statements. Each uses the inbox index, so the engine touches
//     K matching rows rather than issuing a query per member. Cost is O(D)
//     round trips and O(log M + K) work inside each statement.
//   - A following SELECT uses the same predicate and only runs when M is at
//     most unreadSyncCap. Push goes to that result, and only to users the
//     presence service reports as online, in batches of 500.
//   - When M is above unreadSyncCap the UPDATEs run off the request and no
//     per-member list is built. The RPC does not wait on the group size.

func (s *Service) dropDeletedUnread(ctx context.Context, channelID int64, removed []removedMessage, pts int32) {
	if s == nil || s.store == nil || len(removed) == 0 {
		return
	}
	cuts := unreadCuts(removed)
	if len(cuts) == 0 {
		return
	}
	n, err := s.store.CountMembers(ctx, channelID)
	if err != nil {
		logx.WithContext(ctx).Errorf("channel.delete unread count channel=%d: %v", channelID, err)
		n = unreadSyncCap + 1
	}
	if n > unreadSyncCap {
		go s.applyUnreadAsync(channelID, cuts, n)
		return
	}
	rows, err := s.store.ApplyUnreadCuts(ctx, channelID, cuts, true)
	if err != nil {
		logx.WithContext(ctx).Errorf("channel.delete unread channel=%d: %v", channelID, err)
		return
	}
	s.pushDeletedUnread(ctx, channelID, rows, pts)
}

func (s *Service) applyUnreadAsync(channelID int64, cuts []UnreadCut, members int32) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.store.ApplyUnreadCuts(ctx, channelID, cuts, false); err != nil {
		logx.WithContext(ctx).Errorf("channel.delete unread async channel=%d members=%d: %v", channelID, members, err)
	}
}

func unreadCuts(removed []removedMessage) []UnreadCut {
	cuts := make([]UnreadCut, 0, len(removed))
	seen := make(map[int32]struct{}, len(removed))
	for _, m := range removed {
		if m.ID <= 0 {
			continue
		}
		if _, ok := seen[m.ID]; ok {
			continue
		}
		seen[m.ID] = struct{}{}
		cuts = append(cuts, UnreadCut{Below: m.ID, Exclude: m.Sender, Delta: 1})
	}
	return cuts
}

func (s *Service) pushDeletedUnread(ctx context.Context, channelID int64, rows []ReadState, pts int32) {
	h := s.hooks()
	if h.Push == nil || len(rows) == 0 {
		return
	}
	online := map[int64]bool{}
	if h.Online == nil {
		for _, st := range rows {
			online[st.UserID] = true
		}
	} else {
		ids := make([]int64, 0, len(rows))
		for _, st := range rows {
			if st.UserID != 0 {
				ids = append(ids, st.UserID)
			}
		}
		got, err := h.Online(ctx, ids)
		if err != nil {
			logx.WithContext(ctx).Errorf("channel.delete unread presence channel=%d: %v", channelID, err)
			return
		}
		online = got
	}
	for _, st := range rows {
		if !online[st.UserID] {
			continue
		}
		upd := mtproto.MakeTLUpdateReadChannelInbox(&mtproto.Update{
			ChannelId:        channelID,
			MaxId:            st.ReadInboxMaxID,
			StillUnreadCount: st.UnreadCount,
			Pts_INT32:        pts,
		}).To_Update()
		h.Push(ctx, st.UserID, nil, s.pack(ctx, st.UserID, channelID, upd))
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
