package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/account/botconn"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
)

func TestConfirmBotConnectionLocalBot(t *testing.T) {
	botadmin.Reset()
	botconn.Reset()
	created, err := botadmin.CreateBot(context.Background(), 11, false, "Helper", "helperbot", mtproto.MakeTLInputUser(&mtproto.InputUser{
		UserId: userpb.BotFatherId,
	}).To_InputUser())
	if err != nil {
		t.Fatal(err)
	}
	c := &AccountCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: 11}}
	if _, err := c.AccountConfirmBotConnection(nil); status.Convert(err).Message() != "BOT_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{
		BotId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
	}); status.Convert(err).Message() != "BOT_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{
		BotId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: created.GetId(), AccessHash: 1}).To_InputUser(),
	}); status.Convert(err).Message() != "BOT_ID_INVALID" {
		t.Fatal(err)
	}
	ok, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{
		BotId: mtproto.MakeTLInputUser(&mtproto.InputUser{
			UserId: created.GetId(), AccessHash: created.GetAccessHash().GetValue(),
		}).To_InputUser(),
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err, ok)
	}
	if !botconn.Confirmed(context.Background(), 11, created.GetId(), nil) {
		t.Fatal("confirmation was not stored")
	}
	anon := &AccountCore{ctx: context.Background()}
	if _, err := anon.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{}); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
}
