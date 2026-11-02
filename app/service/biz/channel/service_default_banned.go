package channel

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// EditDefaultBanned sets the rights every member of a megagroup is restricted
// by (messages.editChatDefaultBannedRights on a channel peer). Sending and
// info changes already consult ch.DefaultBanned.
func (s *Service) EditDefaultBanned(ctx context.Context, actor, actorAuth int64, in *mtproto.InputChannel, rights *mtproto.ChatBannedRights) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup {
		return nil, mtproto.ErrChatAdminRequired
	}
	if !p.IsCreator() && !p.adminRights().CanBanUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if rights == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	prev := mtproto.BannedRights(ch.DefaultBanned).ToChatBannedRights()
	next := int64(rights.ToBannedRights())
	if next == ch.DefaultBanned {
		return nil, mtproto.ErrChatNotModified
	}
	ch.DefaultBanned = next
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, actor, mtproto.MakeTLChannelAdminLogEventActionDefaultBannedRights(&mtproto.ChannelAdminLogEventAction{
		PrevBannedRights: prev,
		NewBannedRights:  mtproto.BannedRights(next).ToChatBannedRights(),
	}).To_ChannelAdminLogEventAction())
	return s.pack(ctx, actor, ch.ID, channelUpdate(ch.ID)), nil
}
