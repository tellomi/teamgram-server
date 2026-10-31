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
	"google.golang.org/grpc/status"
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

	botCtx := ctxAs(t, user.GetId())
	button, err := svc.BotsGetBotMenuButton(botCtx, &mtproto.TLBotsGetBotMenuButton{})
	if err != nil || button.GetPredicateName() != mtproto.Predicate_botMenuButtonDefault {
		t.Fatalf("default button %v %v", button, err)
	}
	if _, err := svc.BotsSetBotMenuButton(ctx, &mtproto.TLBotsSetBotMenuButton{}); err == nil || status.Convert(err).Message() != "USER_BOT_REQUIRED" {
		t.Fatal(err)
	}
	set, err := svc.BotsSetBotMenuButton(botCtx, &mtproto.TLBotsSetBotMenuButton{
		Button: mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: "Shop", Url: "https://bot.example/shop"}).To_BotMenuButton(),
	})
	if err != nil || !mtproto.FromBool(set) {
		t.Fatal(err)
	}
	button, err = svc.BotsGetBotMenuButton(botCtx, &mtproto.TLBotsGetBotMenuButton{})
	if err != nil || button.GetText() != "Shop" || button.GetUrl() != "https://bot.example/shop" {
		t.Fatalf("custom %#v %v", button, err)
	}
	if _, err := svc.BotsSetBotMenuButton(botCtx, &mtproto.TLBotsSetBotMenuButton{
		Button: mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: "Shop", Url: "http://bot.example/shop"}).To_BotMenuButton(),
	}); err == nil || status.Convert(err).Message() != "BUTTON_URL_INVALID" {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PostMessages: true}).To_ChatAdminRights()
	if _, err := svc.BotsSetBotBroadcastDefaultAdminRights(botCtx, &mtproto.TLBotsSetBotBroadcastDefaultAdminRights{AdminRights: rights}); err != nil {
		t.Fatal(err)
	}
	group, channel := botadmin.DefaultAdminRights(user.GetId())
	if group != nil || channel == nil || !channel.GetPostMessages() {
		t.Fatalf("rights group=%v channel=%v", group, channel)
	}
	if _, err := svc.HelpSetBotUpdatesStatus(botCtx, &mtproto.TLHelpSetBotUpdatesStatus{PendingUpdatesCount: 3, Message: "behind"}); err != nil {
		t.Fatal(err)
	}
	if n, msg, ok := botadmin.UpdatesStatus(user.GetId()); !ok || n != 3 || msg != "behind" {
		t.Fatalf("status %d %q %v", n, msg, ok)
	}
	if _, err := svc.BotsSendCustomRequest(botCtx, &mtproto.TLBotsSendCustomRequest{
		CustomMethod: "ping",
		Params:       mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: "{}"}).To_DataJSON(),
	}); err == nil || status.Convert(err).Message() != "METHOD_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.BotsAnswerWebhookJSONQuery(botCtx, &mtproto.TLBotsAnswerWebhookJSONQuery{
		QueryId: 4,
		Data:    mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: "not-json"}).To_DataJSON(),
	}); err == nil || status.Convert(err).Message() != "DATA_JSON_INVALID" {
		t.Fatal(err)
	}
}
