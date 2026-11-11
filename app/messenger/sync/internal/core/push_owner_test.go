package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	statusclient "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/app/service/status/status"

	"github.com/zeromicro/go-zero/core/logx"
)

// The online-session hash still lists auth key K under user A after the key
// is rebound to user B. An update for A must not be pushed to that session,
// and the stale field must be removed. A's other key still receives it.
func TestRebindDoesNotPushOldUserUpdateToNewSession(t *testing.T) {
	const (
		userA int64 = 11
		userB int64 = 22
		keyA  int64 = 100
		keyB  int64 = 200
		gate        = "10.0.0.8:20120"
	)
	pusher := &capturePusher{}
	st := &routeStatus{sessions: []*status.SessionEntry{
		{UserId: userA, AuthKeyId: keyA, PermAuthKeyId: keyA, Gateway: gate},
		{UserId: userA, AuthKeyId: keyB, PermAuthKeyId: keyB, Gateway: gate},
	}}
	owners := &routeAuth{users: map[int64]int64{keyA: userA, keyB: userB}}
	c := newRouteCore(st, owners, gate, pusher)

	c.pushUpdatesToSession(syncTypeUser, userA, 0, nil, nil, nil, nil, false)

	if len(pusher.updates) != 1 {
		t.Fatalf("pushes = %d, want 1 (only the key still owned by A)", len(pusher.updates))
	}
	if pusher.updates[0].GetPermAuthKeyId() != keyA {
		t.Fatalf("pushed auth key %d, want %d", pusher.updates[0].GetPermAuthKeyId(), keyA)
	}
	if got := session.PushUpdatesUser(pusher.updates[0]); got != userA {
		t.Fatalf("stamped user = %d, want %d", got, userA)
	}
	if len(st.offlineUser) != 1 || st.offlineUser[0] != userA || st.offlineKey[0] != keyB {
		t.Fatalf("evict users %v keys %v, want user %d key %d", st.offlineUser, st.offlineKey, userA, keyB)
	}
}

func TestPushKeepsRouteWhenOwnerLookupFails(t *testing.T) {
	const (
		userA int64 = 11
		keyA  int64 = 100
		gate        = "10.0.0.8:20120"
	)
	pusher := &capturePusher{}
	st := &routeStatus{sessions: []*status.SessionEntry{
		{UserId: userA, AuthKeyId: keyA, PermAuthKeyId: keyA, Gateway: gate},
	}}
	c := newRouteCore(st, &routeAuth{err: errors.New("authsession down")}, gate, pusher)
	c.pushUpdatesToSession(syncTypeUser, userA, 0, nil, nil, nil, nil, false)
	if len(pusher.updates) != 1 {
		t.Fatalf("pushes = %d, want the previous route kept", len(pusher.updates))
	}
	if len(st.offlineKey) != 0 {
		t.Fatalf("lookup error evicted %v", st.offlineKey)
	}
}

func newRouteCore(st statusclient.StatusClient, owners authsessionclient.AuthsessionClient, gate string, pusher dao.SessionPusher) *SyncCore {
	d := &dao.Dao{
		StatusClient:      st,
		AuthsessionClient: owners,
	}
	d.SetSessionPusher(gate, pusher)
	return &SyncCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: d},
		Logger: logx.WithContext(context.Background()),
	}
}

type capturePusher struct {
	updates []*session.TLSessionPushUpdatesData
}

func (c *capturePusher) PushUpdates(_ context.Context, msg *session.TLSessionPushUpdatesData) error {
	c.updates = append(c.updates, msg)
	return nil
}
func (c *capturePusher) PushSessionUpdates(context.Context, *session.TLSessionPushSessionUpdatesData) error {
	return nil
}
func (c *capturePusher) PushRpcResult(context.Context, *session.TLSessionPushRpcResultData) error {
	return nil
}
func (c *capturePusher) Close() error { return nil }

type routeStatus struct {
	sessions    []*status.SessionEntry
	offlineUser []int64
	offlineKey  []int64
}

func (r *routeStatus) StatusGetUserOnlineSessions(_ context.Context, in *status.TLStatusGetUserOnlineSessions) (*status.UserSessionEntryList, error) {
	return &status.UserSessionEntryList{UserId: in.GetUserId(), UserSessions: r.sessions}, nil
}
func (r *routeStatus) StatusSetSessionOffline(_ context.Context, in *status.TLStatusSetSessionOffline) (*mtproto.Bool, error) {
	if in != nil {
		r.offlineUser = append(r.offlineUser, in.GetUserId())
		r.offlineKey = append(r.offlineKey, in.GetAuthKeyId())
	}
	return mtproto.BoolTrue, nil
}
func (r *routeStatus) StatusSetSessionOnline(context.Context, *status.TLStatusSetSessionOnline) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}
func (r *routeStatus) StatusGetUsersOnlineSessionsList(context.Context, *status.TLStatusGetUsersOnlineSessionsList) (*status.Vector_UserSessionEntryList, error) {
	return nil, nil
}
func (r *routeStatus) StatusGetChannelOnlineUsers(context.Context, *status.TLStatusGetChannelOnlineUsers) (*status.Vector_Long, error) {
	return nil, nil
}
func (r *routeStatus) StatusSetUserChannelsOnline(context.Context, *status.TLStatusSetUserChannelsOnline) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeStatus) StatusSetUserChannelsOffline(context.Context, *status.TLStatusSetUserChannelsOffline) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeStatus) StatusSetChannelUserOffline(context.Context, *status.TLStatusSetChannelUserOffline) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeStatus) StatusSetChannelUsersOnline(context.Context, *status.TLStatusSetChannelUsersOnline) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeStatus) StatusSetChannelOffline(context.Context, *status.TLStatusSetChannelOffline) (*mtproto.Bool, error) {
	return nil, nil
}

type routeAuth struct {
	users map[int64]int64
	err   error
}

func (r *routeAuth) AuthsessionGetUserId(_ context.Context, in *authsession.TLAuthsessionGetUserId) (*mtproto.Int64, error) {
	if r != nil && r.err != nil {
		return nil, r.err
	}
	var id int64
	if r != nil && in != nil {
		id = r.users[in.GetAuthKeyId()]
	}
	return &mtproto.Int64{V: id}, nil
}
func (r *routeAuth) AuthsessionGetAuthorizations(context.Context, *authsession.TLAuthsessionGetAuthorizations) (*mtproto.Account_Authorizations, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionResetAuthorization(context.Context, *authsession.TLAuthsessionResetAuthorization) (*authsession.Vector_Long, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetLayer(context.Context, *authsession.TLAuthsessionGetLayer) (*mtproto.Int32, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetLangPack(context.Context, *authsession.TLAuthsessionGetLangPack) (*mtproto.String, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetClient(context.Context, *authsession.TLAuthsessionGetClient) (*mtproto.String, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetLangCode(context.Context, *authsession.TLAuthsessionGetLangCode) (*mtproto.String, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetPushSessionId(context.Context, *authsession.TLAuthsessionGetPushSessionId) (*mtproto.Int64, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetFutureSalts(context.Context, *authsession.TLAuthsessionGetFutureSalts) (*mtproto.FutureSalts, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionQueryAuthKey(context.Context, *authsession.TLAuthsessionQueryAuthKey) (*mtproto.AuthKeyInfo, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionSetAuthKey(context.Context, *authsession.TLAuthsessionSetAuthKey) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionBindAuthKeyUser(context.Context, *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionUnbindAuthKeyUser(context.Context, *authsession.TLAuthsessionUnbindAuthKeyUser) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetPermAuthKeyId(context.Context, *authsession.TLAuthsessionGetPermAuthKeyId) (*mtproto.Int64, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionBindTempAuthKey(context.Context, *authsession.TLAuthsessionBindTempAuthKey) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionSetClientSessionInfo(context.Context, *authsession.TLAuthsessionSetClientSessionInfo) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetAuthorization(context.Context, *authsession.TLAuthsessionGetAuthorization) (*mtproto.Authorization, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionGetAuthStateData(context.Context, *authsession.TLAuthsessionGetAuthStateData) (*authsession.AuthKeyStateData, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionSetLayer(context.Context, *authsession.TLAuthsessionSetLayer) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionSetInitConnection(context.Context, *authsession.TLAuthsessionSetInitConnection) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionSetAndroidPushSessionId(context.Context, *authsession.TLAuthsessionSetAndroidPushSessionId) (*mtproto.Bool, error) {
	return nil, nil
}
func (r *routeAuth) AuthsessionChangeAuthorizationSettings(context.Context, *authsession.TLAuthsessionChangeAuthorizationSettings) (*mtproto.Bool, error) {
	return nil, nil
}

var (
	_ statusclient.StatusClient           = (*routeStatus)(nil)
	_ authsessionclient.AuthsessionClient = (*routeAuth)(nil)
	_ dao.SessionPusher                   = (*capturePusher)(nil)
)
