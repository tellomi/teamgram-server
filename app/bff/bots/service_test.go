package bots

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmeta "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func ctxAs(t *testing.T, userID int64) context.Context {
	t.Helper()
	buf, err := proto.Marshal(&rpcmeta.RpcMetadata{UserId: userID, PermAuthKeyId: 1})
	if err != nil {
		t.Fatal(err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func TestServiceCreateBot(t *testing.T) {
	botadmin.Reset()
	svc := &Service{}
	ctx := ctxAs(t, 11)
	ok, err := svc.BotsCheckUsername(ctx, &mtproto.TLBotsCheckUsername{Username: "wirebot"})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(ok, err)
	}
	user, err := svc.BotsCreateBot(ctx, &mtproto.TLBotsCreateBot{
		Name: "Wire", Username: "wirebot",
		ManagerId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userpb.BotFatherId}).To_InputUser(),
	})
	if err != nil || user.GetUsername().GetValue() != "wirebot" {
		t.Fatal(user, err)
	}
	list, err := svc.BotsGetAdminedBots(ctx, &mtproto.TLBotsGetAdminedBots{})
	if err != nil || len(list.GetDatas()) != 1 {
		t.Fatal(list, err)
	}
}
