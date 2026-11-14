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
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestAddChatUserPrivacyAndBlock(t *testing.T) {
	users := &inviteUser{allow: true}
	chats := &inviteChat{}
	msgs := &inviteMsg{}
	c := &ChatsCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
			ChatClient: chatclient.NewDirectChatClient(chats),
			MsgClient:  msgs,
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	in := &mtproto.TLMessagesAddChatUserF24753E3{
		ChatId: 9,
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 1}).To_InputUser(),
	}

	users.allow = false
	if _, err := c.MessagesAddChatUserF24753E3(in); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("privacy deny: %v", err)
	}
	if chats.adds != 0 {
		t.Fatalf("denied invite still added the user %d", chats.adds)
	}

	users.allow = true
	users.blocked = true
	if _, err := c.MessagesAddChatUserF24753E3(in); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("block deny: %v", err)
	}

	users.blocked = false
	got, err := c.MessagesAddChatUserF24753E3(in)
	if err != nil || got == nil {
		t.Fatalf("allowed invite: %v %v", got, err)
	}
	if chats.adds != 1 || msgs.sends != 1 {
		t.Fatalf("adds %d sends %d", chats.adds, msgs.sends)
	}
}

type inviteUser struct {
	userclient.UserClient
	allow   bool
	blocked bool
}

func (u *inviteUser) UserGetMutableUsers(context.Context, *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	return &user.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 1}},
		{User: &mtproto.UserData{Id: 2}},
	}}, nil
}

func (u *inviteUser) UserCheckPrivacy(context.Context, *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if u.allow {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (u *inviteUser) UserBlockedEitherWay(context.Context, *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	if u.blocked {
		return &user.Vector_Long{Datas: []int64{2}}, nil
	}
	return &user.Vector_Long{}, nil
}

type inviteChat struct {
	chatclient.ChatClient
	adds int
}

func (c *inviteChat) ChatAddChatUser(context.Context, *chatpb.TLChatAddChatUser) (*mtproto.MutableChat, error) {
	c.adds++
	return &mtproto.MutableChat{Chat: &mtproto.ImmutableChat{Id: 9}}, nil
}

type inviteMsg struct {
	msgclient.MsgClient
	sends int
}

func (m *inviteMsg) MsgSendMessageV2(context.Context, *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.sends++
	return mtproto.MakeEmptyUpdates(), nil
}
