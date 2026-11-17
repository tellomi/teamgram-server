package service

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type indexStub struct {
	peerErr  error
	peerOK   *mtproto.Bool
	nameErr  error
	nameOK   *mtproto.Bool
	peerHits int
	nameHits int
}

func (s *indexStub) UserDeleteUsername(context.Context, *user.TLUserDeleteUsername) (*mtproto.Bool, error) {
	s.nameHits++
	return s.nameOK, s.nameErr
}

func (s *indexStub) UserDeleteUsernameByPeer(context.Context, *user.TLUserDeleteUsernameByPeer) (*mtproto.Bool, error) {
	s.peerHits++
	return s.peerOK, s.peerErr
}

func TestDeleteChannelUsernameIndexStopsBeforeChannelClear(t *testing.T) {
	ctx := context.Background()
	if err := deleteChannelUsernameIndex(ctx, nil, 9, []string{"room"}); err == nil {
		t.Fatal("nil index client succeeded")
	}
	stub := &indexStub{peerErr: errors.New("index down"), peerOK: mtproto.BoolFalse}
	if err := deleteChannelUsernameIndex(ctx, stub, 9, []string{"room", "collectible"}); err == nil {
		t.Fatal("peer delete error was ignored")
	}
	if stub.nameHits != 0 {
		t.Fatal("name delete ran after peer delete failed")
	}
	stub = &indexStub{peerOK: mtproto.BoolTrue, nameOK: mtproto.BoolFalse}
	if err := deleteChannelUsernameIndex(ctx, stub, 9, []string{"room"}); err == nil {
		t.Fatal("BoolFalse name delete was ignored")
	}
	stub = &indexStub{peerOK: mtproto.BoolTrue, nameOK: mtproto.BoolTrue}
	if err := deleteChannelUsernameIndex(ctx, stub, 9, []string{"room", "room", ""}); err != nil {
		t.Fatal(err)
	}
	if stub.peerHits != 1 || stub.nameHits != 1 {
		t.Fatalf("hits peer=%d name=%d", stub.peerHits, stub.nameHits)
	}
}
