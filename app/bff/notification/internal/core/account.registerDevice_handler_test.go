package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/push"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestRegisterAndUnregisterDevice(t *testing.T) {
	store := &push.MemoryStore{}
	c := &NotificationCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Devices: store},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 9, PermAuthKeyId: 55, AuthId: 7},
	}
	secret := []byte{9, 8, 7, 6}
	ok, err := c.AccountRegisterDevice(&mtproto.TLAccountRegisterDevice{
		NoMuted:    true,
		TokenType:  push.TokenAPNS,
		Token:      "abc",
		AppSandbox: mtproto.BoolFalse,
		Secret:     secret,
		OtherUids:  []int64{4},
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	voip, err := c.AccountRegisterDevice(&mtproto.TLAccountRegisterDevice{
		TokenType:  push.TokenAPNSVoIP,
		Token:      "voip",
		AppSandbox: mtproto.BoolTrue,
		Secret:     secret,
	})
	if err != nil || !mtproto.FromBool(voip) {
		t.Fatal(err)
	}
	list, err := store.ListByUser(context.Background(), 9)
	if err != nil || len(list) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
	var alert push.Device
	for _, d := range list {
		if d.TokenType == push.TokenAPNS {
			alert = d
		}
	}
	if alert.AuthKeyID != 55 || alert.Token != "abc" || alert.AppSandbox || !alert.NoMuted || string(alert.Secret) != string(secret) || alert.OtherUIDs[0] != 4 {
		t.Fatalf("%+v", alert)
	}
	if _, err := c.AccountRegisterDevice(&mtproto.TLAccountRegisterDevice{TokenType: push.TokenFCM}); err != mtproto.ErrTokenInvalid {
		t.Fatal(err)
	}
	if _, err := c.AccountUnregisterDevice(&mtproto.TLAccountUnregisterDevice{
		TokenType: push.TokenAPNS,
		Token:     "abc",
		OtherUids: []int64{4},
	}); err != nil {
		t.Fatal(err)
	}
	list, _ = store.ListByUser(context.Background(), 9)
	if len(list) != 1 || list[0].TokenType != push.TokenAPNSVoIP {
		t.Fatalf("%+v", list)
	}
}
