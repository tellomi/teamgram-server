package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/sess"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"

	"google.golang.org/grpc/status"
)

func TestUnboundPermSessionIsRejected(t *testing.T) {
	_, err := (&SessionCore{}).getOrFetchMainAuthWrapper(0)
	if status.Convert(err).Message() != "AUTH_KEY_PERM_EMPTY" {
		t.Fatalf("got %v", err)
	}
}

type permEmptyAuth struct {
	authsessionclient.AuthsessionClient
}

func (permEmptyAuth) AuthsessionGetAuthStateData(context.Context, *authsession.TLAuthsessionGetAuthStateData) (*authsession.AuthKeyStateData, error) {
	return nil, mtproto.ErrAuthKeyPermEmpty
}

func TestUnboundTempKeyGetsItsOwnSession(t *testing.T) {
	d := &dao.Dao{AuthsessionClient: permEmptyAuth{}}
	mgr := sess.NewMainAuthWrapperManager(d)
	c := New(context.Background(), &svc.ServiceContext{MainAuthMgr: mgr, Dao: d})

	first, err := c.getOrFetchMainAuthWrapper(88001)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Stop()
	second, err := c.getOrFetchMainAuthWrapper(88002)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Stop()
	if first == nil || second == nil || first == second {
		t.Fatal("unbound temp keys shared one session")
	}
	again, err := c.getOrFetchMainAuthWrapper(88001)
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Fatal("the same temp key opened a second session")
	}
	if _, err = c.getOrFetchMainAuthWrapper(0); status.Convert(err).Message() != "AUTH_KEY_PERM_EMPTY" {
		t.Fatalf("perm id 0: %v", err)
	}
}
