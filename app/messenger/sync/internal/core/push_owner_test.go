package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	statusclient "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/app/service/status/status"

	"github.com/zeromicro/go-zero/core/logx"
)

// Two devices of one user both receive the push, and each message carries
// that user. Sync does not ask authsession on this path.
func TestFanoutStampsExpectedUserOnEveryDevice(t *testing.T) {
	const (
		userA int64 = 11
		key1  int64 = 100
		key2  int64 = 200
		gate        = "10.0.0.8:20120"
	)
	pusher := &capturePusher{}
	st := &routeStatus{sessions: []*status.SessionEntry{
		{UserId: userA, AuthKeyId: key1, PermAuthKeyId: key1, Gateway: gate},
		{UserId: userA, AuthKeyId: key2, PermAuthKeyId: key2, Gateway: gate},
	}}
	c := newRouteCore(st, gate, pusher)
	c.pushUpdatesToSession(syncTypeUser, userA, 0, nil, nil, nil, updatesTooLong(), false)

	if len(pusher.updates) != 2 {
		t.Fatalf("pushes = %d, want both devices", len(pusher.updates))
	}
	got := map[int64]int64{}
	for _, msg := range pusher.updates {
		got[msg.GetPermAuthKeyId()] = session.PushUpdatesUser(msg)
	}
	if got[key1] != userA || got[key2] != userA {
		t.Fatalf("stamped users = %v", got)
	}
	if len(st.offlineKey) != 0 {
		t.Fatalf("fan-out evicted keys %v", st.offlineKey)
	}
}

func newRouteCore(st statusclient.StatusClient, gate string, pusher dao.SessionPusher) *SyncCore {
	d := &dao.Dao{StatusClient: st}
	d.SetSessionPusher(gate, pusher)
	return &SyncCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: d},
		Logger: logx.WithContext(context.Background()),
	}
}

func updatesTooLong() *mtproto.Updates {
	return mtproto.MakeTLUpdatesTooLong(nil).To_Updates()
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

var (
	_ statusclient.StatusClient = (*routeStatus)(nil)
	_ dao.SessionPusher         = (*capturePusher)(nil)
)
