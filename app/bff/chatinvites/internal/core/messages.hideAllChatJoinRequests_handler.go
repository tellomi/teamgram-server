package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/chatinvites/joinhide"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesHideAllChatJoinRequests
// messages.hideAllChatJoinRequests#e085f4ea flags:# approved:flags.0?true peer:InputPeer link:flags.1?string = Updates;
// https://core.telegram.org/method/messages.hideAllChatJoinRequests
func (c *ChatInvitesCore) MessagesHideAllChatJoinRequests(in *mtproto.TLMessagesHideAllChatJoinRequests) (*mtproto.Updates, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	link := ""
	if in.GetLink() != nil {
		link = in.GetLink().GetValue()
	}
	if peer.IsChannel() {
		return joinhide.HideChannel(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, peer, in.GetApproved(), link)
	}
	if !peer.IsChat() || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	mChat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peer.PeerId})
	if err != nil || mChat == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	me, _ := mChat.GetImmutableChatParticipant(c.MD.UserId)
	if me == nil || !me.CanInviteUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	var linkVal *wrapperspb.StringValue
	if link != "" {
		linkVal = &wrapperspb.StringValue{Value: link}
	}
	pending, err := c.svcCtx.Dao.ChatClient.ChatHideChatJoinRequests(c.ctx, &chatpb.TLChatHideChatJoinRequests{
		SelfId:   c.MD.UserId,
		ChatId:   peer.PeerId,
		Approved: in.GetApproved(),
		Link:     linkVal,
	})
	if err != nil {
		return nil, err
	}
	if pending == nil {
		pending = &chatpb.RecentChatInviteRequesters{}
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePendingJoinRequests(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerChat(peer.PeerId),
		RequestsPending:  pending.GetRequestsPending(),
		RecentRequesters: pending.GetRecentRequesters(),
	}).To_Update()), nil
}
