package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestScheduledSendRefusesABlock(t *testing.T) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 9)
	sink := scheduleSink{users: blockUsers{blocked: true}}
	if err := sink.allow(context.Background(), 3, peer); err != mtproto.ErrUserIsBlocked {
		t.Fatalf("blocked: %v", err)
	}
	sink.users = blockUsers{blockedByMe: true}
	if err := sink.allow(context.Background(), 3, peer); err != mtproto.ErrYouBlockedUser {
		t.Fatalf("you blocked: %v", err)
	}
	sink.users = blockUsers{}
	if err := sink.allow(context.Background(), 3, peer); err != nil {
		t.Fatal(err)
	}
	if err := sink.allow(context.Background(), 3, mtproto.MakePeerUtil(mtproto.PEER_USER, 3)); err != nil {
		t.Fatal(err)
	}
}

type blockUsers struct {
	blocked     bool
	blockedByMe bool
}

func (b blockUsers) UserCheckContact(context.Context, *userpb.TLUserCheckContact) (*mtproto.Bool, error) {
	return mtproto.BoolFalse, nil
}

func (b blockUsers) UserBlockedByUser(_ context.Context, in *userpb.TLUserBlockedByUser) (*mtproto.Bool, error) {
	if in.GetUserId() == 9 && b.blocked {
		return mtproto.BoolTrue, nil
	}
	if in.GetUserId() == 3 && b.blockedByMe {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}
