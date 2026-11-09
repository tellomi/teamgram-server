package svc

import (
	"context"
	"errors"
	"testing"

	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	"github.com/teamgram/teamgram-server/pkg/push"
)

func TestRevokeUserSessionsDropsEveryDevice(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 1, UserID: 9, TokenType: push.TokenAPNS, Token: "a"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 2, UserID: 9, TokenType: push.TokenFCM, Token: "b"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 3, UserID: 4, TokenType: push.TokenAPNS, Token: "c"})

	if err := revokeUserSessions(ctx, revokeAuthStub{}, store, 9); err != nil {
		t.Fatal(err)
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 0 {
		t.Fatalf("user 9 %+v", left)
	}
	other, _ := store.ListByUser(ctx, 4)
	if len(other) != 1 {
		t.Fatalf("user 4 %+v", other)
	}
	if err := revokeUserSessions(ctx, revokeAuthStub{}, store, 9); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeUserSessionsKeepsDevicesWhenResetFails(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 1, UserID: 9, TokenType: push.TokenAPNS, Token: "a"})
	err := revokeUserSessions(ctx, revokeAuthStub{err: errors.New("down")}, store, 9)
	if err == nil {
		t.Fatal("expected error")
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 1 {
		t.Fatalf("devices dropped: %+v", left)
	}
}

type revokeAuthStub struct {
	authsessionclient.AuthsessionClient
	err error
}

func (s revokeAuthStub) AuthsessionResetAuthorization(context.Context, *authsession.TLAuthsessionResetAuthorization) (*authsession.Vector_Long, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &authsession.Vector_Long{}, nil
}
