package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetFullUserOpensBuiltinBots(t *testing.T) {
	c := fullUserCore(&fullUserAPI{})
	cases := []int64{mtproto.BotGifId, mtproto.BotVenueId, mtproto.BotFatherId, mtproto.BotBingId}
	for _, id := range cases {
		got, err := c.UsersGetFullUser(fullUserRequest(id))
		if err != nil {
			t.Fatalf("id %d: %v", id, err)
		}
		full := got.GetFullUser()
		if full.GetId() != id || full.GetSettings() == nil || full.GetNotifySettings() == nil || full.GetBotInfo() == nil {
			t.Fatalf("id %d full %+v", id, full)
		}
		if id == mtproto.BotVenueId && full.GetAbout().GetValue() != "搜索地点" {
			t.Fatalf("venue about %q", full.GetAbout().GetValue())
		}
		var bot *mtproto.User
		for _, u := range got.GetUsers() {
			if u.GetId() == id {
				bot = u
			}
		}
		want := catalog.BuiltinBotUser(id)
		if bot == nil || !bot.GetBot() || bot.GetAccessHash().GetValue() == 0 || bot.GetAccessHash().GetValue() != want.GetAccessHash().GetValue() {
			t.Fatalf("id %d user %+v", id, bot)
		}
		buf := mtproto.NewEncodeBuf(512)
		if err = got.Encode(buf, 232); err != nil {
			t.Fatalf("id %d encode: %v", id, err)
		}
		if len(buf.GetBuf()) < 8 {
			t.Fatalf("id %d short encode", id)
		}
	}
}

type emptyUsers struct{ userclient.UserClient }

func (emptyUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
}

func TestGetUsersSynthesizesBuiltinBots(t *testing.T) {
	ctx := context.Background()
	c := &UsersCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: emptyUsers{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	got, err := c.UsersGetUsers(&mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: mtproto.BotFatherId, AccessHash: 1}).To_InputUser(),
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: mtproto.BotGifId, AccessHash: 1}).To_InputUser(),
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 999}).To_InputUser(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	found := map[int64]*mtproto.User{}
	for _, u := range got.GetDatas() {
		found[u.GetId()] = u
	}
	if found[999] != nil {
		t.Fatal("unknown id was invented")
	}
	for _, id := range []int64{mtproto.BotFatherId, mtproto.BotGifId} {
		u := found[id]
		want := catalog.BuiltinBotUser(id)
		if u == nil || !u.GetBot() || u.GetAccessHash().GetValue() != want.GetAccessHash().GetValue() {
			t.Fatalf("id %d %+v", id, u)
		}
	}
}
