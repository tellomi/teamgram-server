package botconn

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/status"
)

type memKV struct {
	m map[string]string
}

func (k memKV) GetCtx(context.Context, string) (string, error) {
	return "", nil
}

func (k memKV) SetCtx(_ context.Context, key, value string) error {
	if k.m == nil {
		return nil
	}
	k.m[key] = value
	return nil
}

func TestTargetAndConfirm(t *testing.T) {
	Reset()
	if _, _, err := Target(1, nil); status.Convert(err).Message() != "BOT_ID_INVALID" {
		t.Fatal(err)
	}
	empty := mtproto.MakeTLInputUserEmpty(&mtproto.InputUser{}).To_InputUser()
	if _, _, err := Target(1, empty); status.Convert(err).Message() != "BOT_ID_INVALID" {
		t.Fatal(err)
	}
	id, hash, err := Target(7, mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 42, AccessHash: 9}).To_InputUser())
	if err != nil || id != 42 || hash != 9 {
		t.Fatalf("id %d %d %v", id, hash, err)
	}
	self, _, err := Target(7, mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser())
	if err != nil || self != 7 {
		t.Fatal(err, self)
	}
	if _, err := Confirm(context.Background(), 0, 42, nil); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	kv := memKV{m: map[string]string{}}
	ok, err := Confirm(context.Background(), 7, 42, kv)
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err, ok)
	}
	if !Confirmed(context.Background(), 7, 42, nil) {
		t.Fatal("not stored")
	}
	if kv.m[rowKey(7, 42)] != "1" {
		t.Fatalf("redis key %#v", kv.m)
	}
	again, err := Confirm(context.Background(), 7, 42, nil)
	if err != nil || !mtproto.FromBool(again) {
		t.Fatal(err)
	}
}
