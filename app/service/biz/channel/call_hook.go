package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// GroupCallAttach is the active voice chat shown on a channel.
type GroupCallAttach struct {
	Call     *mtproto.InputGroupCall
	Active   bool
	NotEmpty bool
}

// LookupGroupCall is set by the phone service. Nil means no voice chat.
var LookupGroupCall func(ctx context.Context, channelID int64) GroupCallAttach

// CallAccess reports whether userID is in the channel and may manage its call.
func (s *Service) CallAccess(ctx context.Context, userID, channelID, accessHash int64) (member, manage bool, err error) {
	if s == nil || s.store == nil {
		return false, false, mtproto.ErrChannelInvalid
	}
	ch, err := s.store.GetChannel(ctx, channelID)
	if err != nil {
		return false, false, err
	}
	if ch == nil || ch.Deleted {
		return false, false, mtproto.ErrChannelInvalid
	}
	if accessHash != 0 && accessHash != ch.AccessHash {
		return false, false, mtproto.ErrChannelInvalid
	}
	p, err := s.store.GetParticipant(ctx, channelID, userID)
	if err != nil {
		return false, false, err
	}
	member = p != nil && p.InChannel()
	if p.IsCreator() {
		return member, true, nil
	}
	if p != nil && p.IsAdmin() && p.adminRights().CanManageCall() {
		return member, true, nil
	}
	return member, false, nil
}

// VisibleChat is the channel object a viewer is allowed to see.
func (s *Service) VisibleChat(ctx context.Context, viewer, channelID int64) *mtproto.Chat {
	if s == nil || s.store == nil {
		return nil
	}
	ch, err := s.store.GetChannel(ctx, channelID)
	if err != nil || ch == nil || ch.Deleted {
		return nil
	}
	return s.ChatFor(ctx, viewer, ch)
}
