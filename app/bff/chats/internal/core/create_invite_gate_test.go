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
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestCreateChatSkipsBlockedAndPrivateMembers(t *testing.T) {
	users := &createGateUser{allow: map[int64]bool{2: false, 3: true, 4: true}, blocked: map[int64]bool{4: true}}
	chats := &createGateChat{}
	c := newCreateCore(users, chats)

	layer, err := c.MessagesCreateChat92CEDDD4(&mtproto.TLMessagesCreateChat92CEDDD4{
		Title: "layer232",
		Users: []*mtproto.InputUser{
			inputCreateUser(2),
			inputCreateUser(3),
			inputCreateUser(4),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ids := missingIDs(layer.GetMissingInvitees()); len(ids) != 2 || ids[0] != 2 || ids[1] != 4 {
		t.Fatalf("missing %v", ids)
	}
	if len(chats.members) != 1 || chats.members[0] != 3 {
		t.Fatalf("created with %v", chats.members)
	}
	if layer.GetUpdates() == nil {
		t.Fatal("layer 232 result has no updates")
	}

	chats.members = nil
	chats.creates = 0
	if _, err = c.MessagesCreateChat9CB126E(&mtproto.TLMessagesCreateChat9CB126E{
		Title: "legacy",
		Users: []*mtproto.InputUser{inputCreateUser(4)},
	}); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("legacy all-blocked: %v", err)
	}
	if chats.creates != 0 {
		t.Fatal("legacy refusal still created the chat")
	}

	got, err := c.MessagesCreateChat9CB126E(&mtproto.TLMessagesCreateChat9CB126E{
		Title: "legacy-ok",
		Users: []*mtproto.InputUser{inputCreateUser(3)},
	})
	if err != nil || got == nil {
		t.Fatalf("allowed legacy create: %v", err)
	}
	if len(chats.members) != 1 || chats.members[0] != 3 {
		t.Fatalf("allowed members %v", chats.members)
	}
}

func TestLegacyCreatePrivacyError(t *testing.T) {
	if err := legacyCreatePrivacyError(false, nil, nil, []*mtproto.MissingInvitee{{UserId: 2}}); err != nil {
		t.Fatal(err)
	}
	if legacyCreatePrivacyError(true, nil, nil, nil) != nil {
		t.Fatal("empty create is not a privacy error")
	}
	if status.Convert(legacyCreatePrivacyError(true, nil, nil, []*mtproto.MissingInvitee{{UserId: 2}})).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatal("legacy all-missing")
	}
	if legacyCreatePrivacyError(true, []int64{3}, nil, []*mtproto.MissingInvitee{{UserId: 2}}) != nil {
		t.Fatal("a partial legacy create still succeeds")
	}
}

func newCreateCore(users *createGateUser, chats *createGateChat) *ChatsCore {
	ctx := context.Background()
	return &ChatsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
			ChatClient: chatclient.NewDirectChatClient(chats),
			MsgClient:  createGateMsg{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
}

func inputCreateUser(id int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: 1}).To_InputUser()
}

func missingIDs(rows []*mtproto.MissingInvitee) []int64 {
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.GetUserId())
	}
	return out
}

type createGateUser struct {
	userclient.UserClient
	allow   map[int64]bool
	blocked map[int64]bool
}

func (u *createGateUser) UserGetMutableUsers(_ context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	rows := make([]*mtproto.ImmutableUser, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		rows = append(rows, &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id}})
	}
	return &user.Vector_ImmutableUser{Datas: rows}, nil
}

func (u *createGateUser) UserGetPrivacy(context.Context, *user.TLUserGetPrivacy) (*user.Vector_PrivacyRule, error) {
	return nil, nil
}

func (u *createGateUser) UserCheckContact(context.Context, *user.TLUserCheckContact) (*mtproto.Bool, error) {
	return mtproto.BoolFalse, nil
}

func (u *createGateUser) UserBlockedEitherWay(_ context.Context, in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	var ids []int64
	for _, id := range in.GetId() {
		if u.blocked[id] {
			ids = append(ids, id)
		}
	}
	return &user.Vector_Long{Datas: ids}, nil
}

func (u *createGateUser) UserCheckPrivacy(_ context.Context, in *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if u.allow[in.GetUserId()] {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

type createGateChat struct {
	chatclient.ChatClient
	creates int
	members []int64
}

func (c *createGateChat) ChatGetUsersChatIdList(context.Context, *chatpb.TLChatGetUsersChatIdList) (*chatpb.Vector_UserChatIdList, error) {
	return nil, nil
}

func (c *createGateChat) ChatCreateChat2(_ context.Context, in *chatpb.TLChatCreateChat2) (*mtproto.MutableChat, error) {
	c.creates++
	c.members = append([]int64{}, in.GetUserIdList()...)
	return &mtproto.MutableChat{Chat: &mtproto.ImmutableChat{Id: 90, Title: in.GetTitle()}}, nil
}

type createGateMsg struct{ msgclient.MsgClient }

func (createGateMsg) MsgSendMessageV2(context.Context, *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	return mtproto.MakeEmptyUpdates(), nil
}
