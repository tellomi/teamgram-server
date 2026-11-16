package channel

import (
	"context"
	"sort"

	"github.com/teamgram/proto/mtproto"
)

// CommonMegagroups is the supergroups both users still belong to.
// Broadcast channels are not common chats. Left members are not included.
func (s *Service) CommonMegagroups(ctx context.Context, self, other int64) ([]*mtproto.Chat, error) {
	if s == nil || s.store == nil || self == 0 || other == 0 || self == other {
		return nil, mtproto.ErrUserIdInvalid
	}
	mine, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	theirs, err := s.store.ListByUser(ctx, other)
	if err != nil {
		return nil, err
	}
	member := map[int64]bool{}
	for _, ch := range theirs {
		if ch != nil && ch.Megagroup && !ch.Broadcast && !ch.Deleted {
			member[ch.ID] = true
		}
	}
	var ids []int64
	byID := map[int64]*Channel{}
	for _, ch := range mine {
		if ch == nil || !ch.Megagroup || ch.Broadcast || ch.Deleted || !member[ch.ID] {
			continue
		}
		ids = append(ids, ch.ID)
		byID[ch.ID] = ch
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*mtproto.Chat, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.ChatFor(ctx, self, byID[id]))
	}
	return out, nil
}
