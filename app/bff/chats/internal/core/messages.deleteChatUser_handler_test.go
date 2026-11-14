package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/chats/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/chats/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestDeleteChatUserPostsServiceMessageBeforeRemoval(t *testing.T) {
	order := []string{}
	chats := &deleteUserChat{chat: basicChat(90, 1, 2), order: &order}
	msgs := &deleteUserMsg{order: &order}
	c := &ChatsCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			ChatClient: chatclient.NewDirectChatClient(chats),
			MsgClient:  msgs,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, Client: "android"},
	}
	up, err := c.MessagesDeleteChatUser(&mtproto.TLMessagesDeleteChatUser{
		ChatId: 90,
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 1}).To_InputUser(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "send" || order[1] != "delete" {
		t.Fatalf("order %v", order)
	}
	if msgs.action.GetPredicateName() != mtproto.Predicate_messageActionChatDeleteUser || msgs.action.GetUserId() != 2 {
		t.Fatalf("action %#v", msgs.action)
	}
	if up == nil || len(up.GetUpdates()) == 0 {
		t.Fatal("android got empty updates")
	}
	if _, err = c.MessagesDeleteChatUser(&mtproto.TLMessagesDeleteChatUser{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(),
	}); status.Convert(err).Message() != "PEER_ID_INVALID" && status.Convert(err).Message() != "USER_NOT_PARTICIPANT" && status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		// chat id 0 fails the pre-check
		if err == nil {
			t.Fatal("missing chat was accepted")
		}
	}
}

func TestChatDeleteAllowed(t *testing.T) {
	chat := basicChat(1, 10, 11)
	if err := chatDeleteAllowed(chat, 10, 11); err != nil {
		t.Fatal(err)
	}
	if status.Convert(chatDeleteAllowed(chat, 11, 10)).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatal("member kicked the creator")
	}
	if err := chatDeleteAllowed(chat, 11, 11); err != nil {
		t.Fatal(err)
	}
}

func basicChat(id, creator, member int64) *mtproto.MutableChat {
	return &mtproto.MutableChat{
		Chat: &mtproto.ImmutableChat{Id: id, Creator: creator},
		ChatParticipants: []*mtproto.ImmutableChatParticipant{
			{UserId: creator, State: mtproto.ChatMemberStateNormal, ParticipantType: mtproto.ChatMemberCreator},
			{UserId: member, State: mtproto.ChatMemberStateNormal, ParticipantType: mtproto.ChatMemberNormal},
		},
	}
}

type deleteUserChat struct {
	chatclient.ChatClient
	chat  *mtproto.MutableChat
	order *[]string
}

func (c *deleteUserChat) ChatGetMutableChat(_ context.Context, in *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	if in.GetChatId() == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return c.chat, nil
}

func (c *deleteUserChat) ChatDeleteChatUser(context.Context, *chatpb.TLChatDeleteChatUser) (*mtproto.MutableChat, error) {
	*c.order = append(*c.order, "delete")
	return c.chat, nil
}

type deleteUserMsg struct {
	msgclient.MsgClient
	action *mtproto.MessageAction
	order  *[]string
}

func (m *deleteUserMsg) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	*m.order = append(*m.order, "send")
	if len(in.GetMessage()) > 0 {
		m.action = in.GetMessage()[0].GetMessage().GetAction()
	}
	return mtproto.MakeUpdatesByUpdates(), nil
}
