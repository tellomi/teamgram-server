package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/chats/owner"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// MessagesGetFutureChatCreatorAfterLeave
// messages.getFutureChatCreatorAfterLeave#3b7d0ea6 peer:InputPeer = User;
// https://core.telegram.org/method/messages.getFutureChatCreatorAfterLeave
func (c *ChatsCore) MessagesGetFutureChatCreatorAfterLeave(in *mtproto.TLMessagesGetFutureChatCreatorAfterLeave) (*mtproto.User, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer.IsChannel() {
		id, err := owner.FutureChannel(c.ctx, c.MD.UserId, peer)
		if err != nil {
			return nil, err
		}
		return owner.User(c.MD.UserId, id), nil
	}
	if !peer.IsChat() || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peer.PeerId})
	if err != nil || chat == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if chat.Creator() != c.MD.UserId {
		return nil, mtproto.ErrChatAdminRequired
	}
	var seats []owner.Seat
	chat.Walk(func(userID int64, p *mtproto.ImmutableChatParticipant) error {
		if p == nil {
			return nil
		}
		seats = append(seats, owner.Seat{
			UserID: userID,
			Admin:  p.IsChatMemberAdmin() || p.IsChatMemberCreator(),
			Date:   p.GetDate(),
			Active: p.IsChatMemberStateNormal(),
		})
		return nil
	})
	id, err := owner.Successor(c.MD.UserId, seats)
	if err != nil {
		return nil, err
	}
	return owner.User(c.MD.UserId, id), nil
}
