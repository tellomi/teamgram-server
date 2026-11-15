package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

func TestPrivateNoForwardsFlagsFollowTheSharedSwitch(t *testing.T) {
	ctx := context.Background()
	if err := chatbox.Shared().SetContentProtected(ctx, 41, 42, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = chatbox.Shared().SetContentProtected(context.Background(), 41, 42, false)
	})

	a := &UsersCore{ctx: ctx, MD: &metadata.RpcMetadata{UserId: 41}}
	full := mtproto.MakeTLUserFull(&mtproto.UserFull{Id: 42}).To_UserFull()
	a.applyPrivateNoForwards(full, 42)
	if !full.GetNoforwardsMyEnabled() || !full.GetNoforwardsPeerEnabled() {
		t.Fatalf("A flags my=%v peer=%v", full.GetNoforwardsMyEnabled(), full.GetNoforwardsPeerEnabled())
	}

	b := &UsersCore{ctx: ctx, MD: &metadata.RpcMetadata{UserId: 42}}
	back := mtproto.MakeTLUserFull(&mtproto.UserFull{Id: 41}).To_UserFull()
	b.applyPrivateNoForwards(back, 41)
	if !back.GetNoforwardsMyEnabled() || !back.GetNoforwardsPeerEnabled() {
		t.Fatalf("B flags my=%v peer=%v", back.GetNoforwardsMyEnabled(), back.GetNoforwardsPeerEnabled())
	}

	self := mtproto.MakeTLUserFull(&mtproto.UserFull{Id: 41}).To_UserFull()
	a.applyPrivateNoForwards(self, 41)
	if self.GetNoforwardsMyEnabled() || self.GetNoforwardsPeerEnabled() {
		t.Fatal("self chat picked up the private-chat switch")
	}

	if err := chatbox.Shared().SetContentProtected(ctx, 42, 41, false); err != nil {
		t.Fatal(err)
	}
	off := mtproto.MakeTLUserFull(&mtproto.UserFull{Id: 42}).To_UserFull()
	a.applyPrivateNoForwards(off, 42)
	if off.GetNoforwardsMyEnabled() || off.GetNoforwardsPeerEnabled() {
		t.Fatal("flags stayed on after the switch was cleared")
	}
}
