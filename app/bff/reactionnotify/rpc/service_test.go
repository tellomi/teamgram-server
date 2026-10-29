package rpc

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/reactionnotify"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func settings(previews bool) *mtproto.ReactionsNotifySettings {
	show := mtproto.BoolFalse
	if previews {
		show = mtproto.BoolTrue
	}
	return mtproto.MakeTLReactionsNotifySettings(&mtproto.ReactionsNotifySettings{
		Sound:              mtproto.MakeTLNotificationSoundNone(nil).To_NotificationSound(),
		ShowPreviews:       show,
		MessagesNotifyFrom: mtproto.MakeTLReactionNotificationsFromAll(nil).To_ReactionNotificationsFrom(),
	}).To_ReactionsNotifySettings()
}

func TestReactionNotifyPersistsPerUser(t *testing.T) {
	reactionnotify.Use(nil)
	svc := New()
	ctx := withUser(4)
	other := withUser(5)

	if _, err := svc.AccountSetReactionsNotifySettings(context.Background(), &mtproto.TLAccountSetReactionsNotifySettings{
		Settings: settings(true),
	}); err == nil {
		t.Fatal("write without a user must fail")
	}
	if _, err := svc.AccountSetReactionsNotifySettings(ctx, &mtproto.TLAccountSetReactionsNotifySettings{}); err == nil {
		t.Fatal("incomplete settings must fail")
	}

	got, err := svc.AccountSetReactionsNotifySettings(ctx, &mtproto.TLAccountSetReactionsNotifySettings{
		Settings: settings(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !mtproto.FromBool(got.GetShowPreviews()) {
		t.Fatal("set should return the stored previews flag")
	}
	again, err := svc.AccountGetReactionsNotifySettings(ctx, &mtproto.TLAccountGetReactionsNotifySettings{})
	if err != nil || !mtproto.FromBool(again.GetShowPreviews()) {
		t.Fatalf("get: %v previews=%v", err, again.GetShowPreviews())
	}
	def, err := svc.AccountGetReactionsNotifySettings(other, &mtproto.TLAccountGetReactionsNotifySettings{})
	if err != nil || mtproto.FromBool(def.GetShowPreviews()) {
		t.Fatal("another user must still see the default")
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := again.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}
