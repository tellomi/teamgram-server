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

func TestUpdateDeviceLockedStoresPeriod(t *testing.T) {
	store := &push.MemoryStore{}
	ctx := context.Background()
	if err := store.Upsert(ctx, push.Device{AuthKeyID: 55, UserID: 9, TokenType: push.TokenAPNS, Token: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, push.Device{AuthKeyID: 8, UserID: 9, TokenType: push.TokenAPNS, Token: "other"}); err != nil {
		t.Fatal(err)
	}
	c := &NotificationCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Devices: store},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 9, PermAuthKeyId: 55},
	}
	ok, err := c.AccountUpdateDeviceLocked(&mtproto.TLAccountUpdateDeviceLocked{Period: 60})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	if _, err := c.AccountRegisterDevice(&mtproto.TLAccountRegisterDevice{
		TokenType:  push.TokenAPNS,
		Token:      "abc",
		AppSandbox: mtproto.BoolFalse,
		Secret:     []byte{1},
	}); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range list {
		switch d.AuthKeyID {
		case 55:
			if d.LockedPeriod != 60 {
				t.Fatalf("locked period %d", d.LockedPeriod)
			}
		case 8:
			if d.LockedPeriod != 0 {
				t.Fatalf("other auth key period %d", d.LockedPeriod)
			}
		}
	}
	if _, err := c.AccountUpdateDeviceLocked(&mtproto.TLAccountUpdateDeviceLocked{Period: 0}); err != nil {
		t.Fatal(err)
	}
	list, _ = store.ListByUser(ctx, 9)
	for _, d := range list {
		if d.AuthKeyID == 55 && d.LockedPeriod != 0 {
			t.Fatalf("period not cleared: %d", d.LockedPeriod)
		}
	}
}
