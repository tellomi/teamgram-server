package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSuggestBirthdayServiceMessage(t *testing.T) {
	msg := &bdayMsg{}
	c := newBirthdayCore(&bdayUsers{mutual: true}, msg)
	ups, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 20}).To_InputUser(),
		Birthday: &mtproto.Birthday{Day: 29, Month: 2, Year: wrapperspb.Int32(2000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.last == nil || msg.last.PeerId != 20 || msg.last.UserId != 10 {
		t.Fatalf("send %+v", msg.last)
	}
	act := msg.last.GetMessage()[0].GetMessage().GetAction()
	if act.GetPredicateName() != mtproto.Predicate_messageActionSuggestBirthday || act.GetBirthday().GetDay() != 29 || act.GetBirthday().GetMonth() != 2 || act.GetBirthday().GetYear().GetValue() != 2000 {
		t.Fatalf("action %#v", act)
	}
	if ups == nil || len(ups.GetUpdates()) == 0 {
		t.Fatal("empty updates")
	}

	if _, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 20}).To_InputUser(),
		Birthday: &mtproto.Birthday{Day: 31, Month: 2},
	}); err != mtproto.ErrBirthdayInvalid {
		t.Fatalf("feb 31: %v", err)
	}
	if _, err := newBirthdayCore(&bdayUsers{}, msg).UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 20}).To_InputUser(),
		Birthday: &mtproto.Birthday{Day: 1, Month: 1},
	}); err != mtproto.ErrUserNotMutualContact {
		t.Fatalf("not mutual: %v", err)
	}
	if _, err := newBirthdayCore(&bdayUsers{mutual: true, bot: true}, msg).UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 20}).To_InputUser(),
		Birthday: &mtproto.Birthday{Day: 1, Month: 1},
	}); err != mtproto.ErrUserIdInvalid {
		t.Fatalf("bot: %v", err)
	}
}

func newBirthdayCore(users user_client.UserClient, msg msg_client.MsgClient) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users, MsgClient: msg}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 10, PermAuthKeyId: 4},
	}
}

type bdayUsers struct {
	user_client.UserClient
	mutual bool
	bot    bool
}

func (b bdayUsers) UserCheckContact(context.Context, *userpb.TLUserCheckContact) (*mtproto.Bool, error) {
	return mtproto.ToBool(b.mutual), nil
}

func (b bdayUsers) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	u := &mtproto.UserData{Id: 20}
	if b.bot {
		u.Bot = &mtproto.BotData{}
	}
	return &mtproto.ImmutableUser{User: u}, nil
}

type bdayMsg struct {
	msg_client.MsgClient
	last *msgpb.TLMsgSendMessageV2
}

func (m *bdayMsg) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	m.last = in
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: in.GetMessage()[0].GetMessage(),
	}).To_Update()), nil
}
