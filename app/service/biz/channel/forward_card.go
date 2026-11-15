package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ForwardCard is the channel object a viewer needs in order to render
// "forwarded from". A member, and a non-member of a public channel, get the
// normal card. A non-member of a private channel gets a min channel so the
// title is still visible. A missing channel is nil.
func (s *Service) ForwardCard(ctx context.Context, viewer, id int64) *mtproto.Chat {
	if s == nil || s.store == nil || id == 0 {
		return nil
	}
	ch, err := s.store.GetChannel(ctx, id)
	if err != nil || ch == nil || ch.Deleted {
		return nil
	}
	card := s.ChatFor(ctx, viewer, ch)
	if card.GetPredicateName() == mtproto.Predicate_channel {
		return card
	}
	return minChannelCard(ch)
}

// ForwardCards loads ForwardCard for each id and drops the misses.
func (s *Service) ForwardCards(ctx context.Context, viewer int64, ids ...int64) []*mtproto.Chat {
	if s == nil || len(ids) == 0 {
		return nil
	}
	out := make([]*mtproto.Chat, 0, len(ids))
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if card := s.ForwardCard(ctx, viewer, id); card != nil {
			out = append(out, card)
		}
	}
	return out
}

// AttachForwardCards adds a forward-source channel that the chat list does
// not already contain as a channel. channelForbidden is replaced, because
// clients look up the name on constructor channel.
func (s *Service) AttachForwardCards(ctx context.Context, viewer int64, messages []*mtproto.Message, chats []*mtproto.Chat) []*mtproto.Chat {
	if s == nil {
		return chats
	}
	index := map[int64]int{}
	for i, ch := range chats {
		if ch != nil && ch.GetId() != 0 {
			index[ch.GetId()] = i
		}
	}
	for _, id := range forwardChannelIDs(messages) {
		if i, ok := index[id]; ok && chats[i].GetPredicateName() == mtproto.Predicate_channel {
			continue
		}
		card := s.ForwardCard(ctx, viewer, id)
		if card == nil {
			continue
		}
		if i, ok := index[id]; ok {
			chats[i] = card
			continue
		}
		chats = append(chats, card)
		index[id] = len(chats) - 1
	}
	return chats
}

func forwardChannelIDs(messages []*mtproto.Message) []int64 {
	var ids []int64
	seen := map[int64]struct{}{}
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, m := range messages {
		fwd := m.GetFwdFrom()
		if fwd == nil {
			continue
		}
		if fwd.GetFromId().GetChannelId() != 0 {
			add(fwd.GetFromId().GetChannelId())
		}
		if fwd.GetSavedFromPeer().GetChannelId() != 0 {
			add(fwd.GetSavedFromPeer().GetChannelId())
		}
	}
	return ids
}

func minChannelCard(ch *Channel) *mtproto.Chat {
	return mtproto.MakeTLChannel(&mtproto.Chat{
		Min:                  true,
		Broadcast:            ch.Broadcast,
		Megagroup:            ch.Megagroup,
		Id:                   ch.ID,
		AccessHash_FLAGINT64: &wrapperspb.Int64Value{Value: ch.AccessHash},
		Title:                ch.Title,
		Username:             optionalString(ch.Username),
		Photo:                chatPhoto(ch.PhotoID),
		Date:                 ch.Date,
	}).To_Chat()
}
