package core

import (
	"math/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/chats/owner"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
)

// MessagesEditChatCreator
// messages.editChatCreator#f743b857 peer:InputPeer user_id:InputUser password:InputCheckPasswordSRP = Updates;
// https://core.telegram.org/method/messages.editChatCreator
func (c *ChatsCore) MessagesEditChatCreator(in *mtproto.TLMessagesEditChatCreator) (*mtproto.Updates, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if in.GetUserId() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	user := mtproto.FromInputUser(c.MD.UserId, in.GetUserId())
	if peer.IsChannel() {
		return owner.EditChannel(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, peer, user.PeerId, in.GetPassword())
	}
	if !peer.IsChat() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if user.PeerType != mtproto.PEER_USER || user.PeerId == 0 || user.PeerId == c.MD.UserId {
		return nil, mtproto.ErrUserIdInvalid
	}
	if owner.PasswordMissing(in.GetPassword()) {
		return nil, mtproto.ErrPasswordMissing
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	chat, err := chat_client.EditChatCreator(c.ctx, c.svcCtx.Dao.ChatClient.Client(), &chatpb.TLChatEditChatAdmin{
		ChatId:          peer.PeerId,
		OperatorId:      c.MD.UserId,
		EditChatAdminId: user.PeerId,
	})
	if err != nil {
		return nil, err
	}
	action := mtproto.MakeTLMessageActionChangeCreator(&mtproto.MessageAction{NewCreatorId: user.PeerId}).To_MessageAction()
	if c.svcCtx.Dao.MsgClient != nil {
		ups, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
			UserId:    c.MD.UserId,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  mtproto.PEER_CHAT,
			PeerId:    peer.PeerId,
			Message: []*msgpb.OutboxMessage{
				msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
					NoWebpage: true,
					RandomId:  rand.Int63(),
					Message:   chat.MakeMessageService(c.MD.UserId, action),
				}).To_OutboxMessage(),
			},
		})
		if err != nil {
			return nil, err
		}
		return ups, nil
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateChatParticipants(&mtproto.Update{
		Participants_CHATPARTICIPANTS: chat.ToChatParticipants(c.MD.UserId),
	}).To_Update()), nil
}
