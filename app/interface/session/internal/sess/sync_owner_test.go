package sess

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// Rebind the permanent key from user A to user B. The status hash can still
// name that key under A, and sync will push A's update at the key. B's
// session must not receive it. B's own update still has to.
func TestRebindKeyUpdateForADoesNotReachBSession(t *testing.T) {
	const (
		userA int64 = 11
		userB int64 = 22
	)
	rec := &recordingStatus{}
	m := newOwnerWrapper(rec, userB, &fakeAuthsessionClient{userID: userB})
	sess := onlineGeneric(m)

	deliverSync(m, userA)
	if sess.outQueue.oMsgs.Len() != 0 {
		t.Fatal("update for A reached the session now bound to B")
	}
	if len(rec.offlineUser) != 1 || rec.offlineUser[0] != userA || rec.offline[0] != m.authKeyId {
		t.Fatalf("stale route not evicted: users %v keys %v", rec.offlineUser, rec.offline)
	}

	deliverSync(m, userB)
	if sess.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("update for B queue = %d, want 1", sess.outQueue.oMsgs.Len())
	}
	if len(rec.offlineUser) != 1 {
		t.Fatalf("B's update evicted a route: %v", rec.offlineUser)
	}
}

// The same in-memory wrapper keeps A's id (the original changeAuthState
// rule) while authsession already says the key belongs to B. An update that
// arrives with no expected user, the way a push looked before this change,
// still must not be written to the connection.
func TestOwnerChangeDropsInFlightUpdateAndEvictsOldRoute(t *testing.T) {
	const (
		userA int64 = 11
		userB int64 = 22
	)
	rec := &recordingStatus{}
	fake := &fakeAuthsessionClient{userID: userB}
	m := newOwnerWrapper(rec, userA, fake)
	sess := onlineGeneric(m)

	m.changeAuthState(context.Background(), mtproto.AuthStateNormal, userB)
	if m.AuthUserId != userA {
		t.Fatalf("wrapper user = %d, want the original %d", m.AuthUserId, userA)
	}
	if len(rec.offlineUser) != 1 || rec.offlineUser[0] != userA {
		t.Fatalf("rebind did not evict A's route: %v", rec.offlineUser)
	}

	deliverSync(m, 0)
	if sess.outQueue.oMsgs.Len() != 0 {
		t.Fatal("in-flight update for A reached the connection after the key was rebound to B")
	}
}

func TestLogoutEvictsSyncRoute(t *testing.T) {
	const userA int64 = 11
	rec := &recordingStatus{}
	m := newOwnerWrapper(rec, userA, nil)
	m.changeAuthState(context.Background(), mtproto.AuthStateLogout, int64(0))
	if len(rec.offlineUser) != 1 || rec.offlineUser[0] != userA || rec.offline[0] != m.authKeyId {
		t.Fatalf("logout left the sync route: users %v keys %v", rec.offlineUser, rec.offline)
	}
}

func TestSetOnlineDoesNotRepublishReboundKey(t *testing.T) {
	const (
		userA int64 = 11
		userB int64 = 22
	)
	rec := &recordingStatus{}
	m := newOwnerWrapper(rec, userA, &fakeAuthsessionClient{userID: userB})
	m.setOnline(context.Background())
	if len(rec.onlineUser) != 0 {
		t.Fatalf("setOnline registered the key under %v", rec.onlineUser)
	}
	if len(rec.offlineUser) != 1 || rec.offlineUser[0] != userA {
		t.Fatalf("stale route not evicted: %v", rec.offlineUser)
	}

	rec.offlineUser = nil
	m.cb.Dao.AuthsessionClient = &fakeAuthsessionClient{userID: userA}
	m.onlineExpired = 0
	m.setOnline(context.Background())
	if len(rec.onlineUser) != 1 || rec.onlineUser[0] != userA {
		t.Fatalf("owned key was not registered: %v", rec.onlineUser)
	}
	if len(rec.offlineUser) != 0 {
		t.Fatalf("owned key was evicted: %v", rec.offlineUser)
	}
}

func TestChangeAuthStateDefaultDoesNotDropOnline(t *testing.T) {
	// These are the states with no explicit case. Production does not pass
	// them to MainAuthWrapper.changeAuthState: temp-key bind uses
	// SessionList.changeAuthState, and reconnect reloads through
	// applyAuthStateData.
	states := []int{
		mtproto.AuthStateNew,
		mtproto.AuthStatePermBound,
		mtproto.AuthStateWaitInit,
		mtproto.AuthStateUnauthorized,
		99,
	}
	for _, state := range states {
		rec := &recordingStatus{}
		m := newOwnerWrapper(rec, 11, nil)
		m.changeAuthState(context.Background(), state, int64(0))
		if len(rec.offline) != 0 || len(rec.offlineUser) != 0 {
			t.Fatalf("state %d deleted online: keys %v users %v", state, rec.offline, rec.offlineUser)
		}
		if m.AuthUserId != 0 {
			t.Fatalf("state %d left user %d", state, m.AuthUserId)
		}
	}
}

func TestLoginReconnectAndTempBindKeepOnline(t *testing.T) {
	ctx := context.Background()

	rec := &recordingStatus{}
	loggingIn := newOwnerWrapper(rec, 0, nil)
	loggingIn.changeAuthState(ctx, mtproto.AuthStateNormal, int64(11))
	if loggingIn.AuthUserId != 11 || len(rec.offline) != 0 {
		t.Fatalf("login user %d offline %v", loggingIn.AuthUserId, rec.offline)
	}

	rec = &recordingStatus{}
	again := newOwnerWrapper(rec, 11, nil)
	again.state = mtproto.AuthStateUnauthorized
	again.applyAuthStateData(ctx, authsession.MakeTLAuthKeyStateData(&authsession.AuthKeyStateData{
		KeyState: mtproto.AuthStateNormal,
		UserId:   11,
	}).To_AuthKeyStateData())
	if again.AuthUserId != 11 || again.state != mtproto.AuthStateNormal || len(rec.offline) != 0 {
		t.Fatalf("reconnect user %d state %d offline %v", again.AuthUserId, again.state, rec.offline)
	}

	rec = &recordingStatus{}
	temp := newOwnerWrapper(rec, 11, nil)
	temp.mainAuth.changeAuthState(mtproto.AuthStatePermBound)
	if temp.AuthUserId != 11 || temp.mainAuth.state != mtproto.AuthStatePermBound || len(rec.offline) != 0 {
		t.Fatalf("temp bind user %d list state %d offline %v", temp.AuthUserId, temp.mainAuth.state, rec.offline)
	}
}

func TestSameUserTwoDevicesBothReceive(t *testing.T) {
	phone := newOwnerWrapper(nil, 11, nil)
	desktop := newOwnerWrapper(nil, 11, nil)
	a := onlineGeneric(phone)
	b := onlineGeneric(desktop)
	deliverSync(phone, 11)
	deliverSync(desktop, 11)
	if a.outQueue.oMsgs.Len() != 1 || b.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("queues phone %d desktop %d", a.outQueue.oMsgs.Len(), b.outQueue.oMsgs.Len())
	}
}

func TestSameDeviceTwoAccountsEachReceiveOwnUpdate(t *testing.T) {
	accountA := newOwnerWrapper(nil, 11, nil)
	accountB := newOwnerWrapper(nil, 22, nil)
	a := onlineGeneric(accountA)
	b := onlineGeneric(accountB)
	deliverSync(accountA, 11)
	deliverSync(accountB, 22)
	deliverSync(accountB, 11)
	if a.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("account A queue %d", a.outQueue.oMsgs.Len())
	}
	if b.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("account B queue %d, want only its own update", b.outQueue.oMsgs.Len())
	}
}

func TestReconnectThenUpdateArrives(t *testing.T) {
	m := newOwnerWrapper(nil, 11, nil)
	sess := onlineGeneric(m)
	ctx := context.Background()
	sess.changeConnState(ctx, kStateOffline)
	if sess.sessionOnline() {
		t.Fatal("socket was still online")
	}
	sess.changeConnState(ctx, kStateOnline)
	if !sess.sessionOnline() {
		t.Fatal("reconnect left the session offline")
	}
	deliverSync(m, 11)
	if sess.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("queue after reconnect %d", sess.outQueue.oMsgs.Len())
	}
}

func TestSyncLookupErrorDoesNotDropMatchingUser(t *testing.T) {
	const userB int64 = 22
	m := newOwnerWrapper(nil, userB, &fakeAuthsessionClient{userErr: errors.New("authsession down")})
	sess := onlineGeneric(m)
	deliverSync(m, userB)
	if sess.outQueue.oMsgs.Len() != 1 {
		t.Fatal("lookup error dropped an update for the session user")
	}
	deliverSync(m, 11)
	if sess.outQueue.oMsgs.Len() != 1 {
		t.Fatal("lookup error delivered another user's update onto this session")
	}
}

func newOwnerWrapper(rec *recordingStatus, userID int64, fake *fakeAuthsessionClient) *MainAuthWrapper {
	m := newTestWrapper(rec)
	m.AuthUserId = userID
	m.cb.Dao.AuthsessionClient = fake
	return m
}

func onlineGeneric(m *MainAuthWrapper) *session {
	sess := onlineSession(m, m.mainAuth, 1)
	sess.isGeneric = true
	m.mainUpdatesSession = sess
	return sess
}

func deliverSync(m *MainAuthWrapper, expected int64) {
	m.onSyncData(context.Background(), &syncData{
		expectedUserId: expected,
		data:           &messageData{obj: mtproto.MakeTLUpdatesTooLong(nil)},
	})
}
