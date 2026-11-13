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
	member, manage, _, _, err = s.callAccess(ctx, userID, channelID, accessHash)
	return member, manage, err
}

// CallJoin is the phone.joinGroupCall view of a channel. public is true when
// the channel has a username, so someone who never joined, or who left, may
// still enter its voice chat. blocked is a kick or a view_messages ban: that
// person may not enter even when the channel is public. A zero access hash
// skips the hash check; the call id was already resolved by the phone service.
func (s *Service) CallJoin(ctx context.Context, userID, channelID int64) (member, manage, public, blocked bool, err error) {
	return s.callAccess(ctx, userID, channelID, 0)
}

func (s *Service) callAccess(ctx context.Context, userID, channelID, accessHash int64) (member, manage, public, blocked bool, err error) {
	if s == nil || s.store == nil {
		return false, false, false, false, mtproto.ErrChannelInvalid
	}
	ch, err := s.store.GetChannel(ctx, channelID)
	if err != nil {
		return false, false, false, false, err
	}
	if ch == nil || ch.Deleted {
		return false, false, false, false, mtproto.ErrChannelInvalid
	}
	if accessHash != 0 && accessHash != ch.AccessHash {
		return false, false, false, false, mtproto.ErrChannelInvalid
	}
	public = ch.Public()
	p, err := s.store.GetParticipant(ctx, channelID, userID)
	if err != nil {
		return false, false, false, false, err
	}
	member = p != nil && p.InChannel()
	if view, _ := s.viewBlocked(p); view || (p != nil && p.State == StateKicked) {
		blocked = true
	}
	if p.IsCreator() {
		return member, true, public, blocked, nil
	}
	if p != nil && p.IsAdmin() && p.adminRights().CanManageCall() {
		return member, true, public, blocked, nil
	}
	return member, false, public, blocked, nil
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
