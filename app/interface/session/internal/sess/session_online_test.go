package sess

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/status/status"

	"github.com/zeromicro/go-zero/core/syncx"
)

// A second generic session stays up while one updates socket closes. The
// user must stay online and the push target must stay in place.
func TestOfflineWaitsForEveryGenericConnection(t *testing.T) {
	rec := &recordingStatus{}
	m := newTestWrapper(rec)

	updates := onlineSession(m, m.mainAuth, 1)
	updates.isGeneric = true
	m.mainUpdatesSession = updates
	other := onlineSession(m, m.tempAuth, 2)
	other.isGeneric = true

	updates.changeConnState(context.Background(), kStateOffline)
	if len(rec.offline) != 0 {
		t.Fatal("marked offline while another connection is still up")
	}
	if m.mainUpdatesSession != updates {
		t.Fatal("push target was dropped")
	}
	if !other.sessionOnline() {
		t.Fatal("the other connection was taken down")
	}

	other.changeConnState(context.Background(), kStateOffline)
	if len(rec.offline) != 1 || rec.offline[0] != m.authKeyId {
		t.Fatalf("offline calls %v", rec.offline)
	}
	if m.mainUpdatesSession != nil {
		t.Fatal("push target kept after the last connection closed")
	}
}

func TestTempRebindKeepsPermUpdatesSession(t *testing.T) {
	m := newTestWrapper(nil)
	perm := onlineSession(m, m.mainAuth, 11)
	perm.isGeneric = true
	m.mainUpdatesSession = perm
	m.tempAuth.authId = 50

	m.resetAuth(mtproto.AuthKeyTypeTemp, 51)
	if m.mainUpdatesSession != perm {
		t.Fatal("temp-key rebind cleared the perm-key updates session")
	}
	if !perm.sessionOnline() {
		t.Fatal("perm connection was marked offline by the temp rebind")
	}
}

func TestPermRebindKeepsTempUpdatesSession(t *testing.T) {
	m := newTestWrapper(nil)
	temp := onlineSession(m, m.tempAuth, 22)
	temp.isGeneric = true
	m.mainUpdatesSession = temp
	m.mainAuth.authId = 10

	m.resetAuth(mtproto.AuthKeyTypePerm, 11)
	if m.mainUpdatesSession != temp {
		t.Fatal("perm-key rebind cleared the temp-key updates session")
	}
}

func TestRebindRetargetsWhenUpdatesSessionIsReplaced(t *testing.T) {
	m := newTestWrapper(nil)
	temp := onlineSession(m, m.tempAuth, 22)
	temp.isGeneric = true
	m.mainUpdatesSession = temp
	perm := onlineSession(m, m.mainAuth, 11)
	perm.isGeneric = true
	m.tempAuth.authId = 50

	m.resetAuth(mtproto.AuthKeyTypeTemp, 77)
	if m.mainUpdatesSession != perm {
		t.Fatal("rebind did not move the push target onto the connection that is still up")
	}
	if _, ok := m.tempAuth.sessions[22]; ok {
		t.Fatal("replaced temp session is still in the map")
	}
}

func TestRebindDropsPushTargetWhenNothingRemains(t *testing.T) {
	m := newTestWrapper(nil)
	temp := onlineSession(m, m.tempAuth, 22)
	temp.isGeneric = true
	m.mainUpdatesSession = temp
	m.tempAuth.authId = 50

	m.resetAuth(mtproto.AuthKeyTypeTemp, 78)
	if m.mainUpdatesSession != nil {
		t.Fatal("push target kept after its key was replaced and no connection remains")
	}
}

func TestUpdatesGoToEveryGenericSession(t *testing.T) {
	m := newTestWrapper(nil)
	first := onlineSession(m, m.mainAuth, 1)
	first.isGeneric = true
	first.canSync = true
	m.mainUpdatesSession = first
	second := onlineSession(m, m.tempAuth, 2)
	second.isGeneric = true
	second.canSync = true
	file := onlineSession(m, m.mediaTempAuth, 3)

	m.onSyncData(context.Background(), &syncData{
		data: &messageData{obj: mtproto.MakeTLUpdatesTooLong(nil)},
	})

	if first.outQueue.oMsgs.Len() == 0 || second.outQueue.oMsgs.Len() == 0 {
		t.Fatal("a generic session received nothing")
	}
	if file.outQueue.oMsgs.Len() != 0 {
		t.Fatal("file connection received an update")
	}
}

func TestFileConnectionDoesNotHoldOnline(t *testing.T) {
	rec := &recordingStatus{}
	m := newTestWrapper(rec)
	main := onlineSession(m, m.mainAuth, 1)
	main.isGeneric = true
	m.mainUpdatesSession = main
	file := onlineSession(m, m.mediaTempAuth, 3)

	file.changeConnState(context.Background(), kStateOffline)
	if len(rec.offline) != 0 {
		t.Fatal("closing a file socket marked the user offline")
	}
	file.connState = kStateOnline
	main.changeConnState(context.Background(), kStateOffline)
	if len(rec.offline) != 1 {
		t.Fatal("user stayed online on a file socket alone")
	}
	if file.outQueue.oMsgs.Len() != 0 {
		t.Fatal("file socket was given updates")
	}
}

func TestRebindDoesNotPromoteFileSession(t *testing.T) {
	m := newTestWrapper(nil)
	temp := onlineSession(m, m.tempAuth, 22)
	temp.isGeneric = true
	m.mainUpdatesSession = temp
	_ = onlineSession(m, m.mediaTempAuth, 33)
	m.tempAuth.authId = 50

	m.resetAuth(mtproto.AuthKeyTypeTemp, 79)
	if m.mainUpdatesSession != nil {
		t.Fatal("file socket became the updates session")
	}
}

func newTestWrapper(rec *recordingStatus) *MainAuthWrapper {
	d := &dao.Dao{}
	if rec != nil {
		d.StatusClient = rec
	}
	m := &MainAuthWrapper{
		authKeyId:  469193,
		AuthUserId: 7,
		cb:         &MainAuthWrapperManager{Dao: d},
		running:    syncx.NewAtomicBool(),
	}
	m.tempAuth = newSessionList(mtproto.AuthKeyTypeTemp, m)
	m.mediaTempAuth = newSessionList(mtproto.AuthKeyTypeMediaTemp, m)
	m.mainAuth = newSessionList(mtproto.AuthKeyTypePerm, m)
	m.mainAuth.authId = m.authKeyId
	return m
}

func onlineSession(m *MainAuthWrapper, list *SessionList, id int64) *session {
	sess := newSession(id, list)
	sess.connState = kStateOnline
	list.sessions[id] = sess
	if list.authId == 0 {
		list.authId = id
	}
	return sess
}

type recordingStatus struct {
	offline     []int64
	offlineUser []int64
	onlineUser  []int64
}

func (r *recordingStatus) StatusSetSessionOnline(_ context.Context, in *status.TLStatusSetSessionOnline) (*mtproto.Bool, error) {
	if in != nil {
		r.onlineUser = append(r.onlineUser, in.GetUserId())
	}
	return mtproto.BoolTrue, nil
}

func (r *recordingStatus) StatusSetSessionOffline(_ context.Context, in *status.TLStatusSetSessionOffline) (*mtproto.Bool, error) {
	if in != nil {
		r.offline = append(r.offline, in.GetAuthKeyId())
		r.offlineUser = append(r.offlineUser, in.GetUserId())
	}
	return mtproto.BoolTrue, nil
}

func (r *recordingStatus) StatusGetUserOnlineSessions(context.Context, *status.TLStatusGetUserOnlineSessions) (*status.UserSessionEntryList, error) {
	return nil, nil
}

func (r *recordingStatus) StatusGetUsersOnlineSessionsList(context.Context, *status.TLStatusGetUsersOnlineSessionsList) (*status.Vector_UserSessionEntryList, error) {
	return nil, nil
}

func (r *recordingStatus) StatusGetChannelOnlineUsers(context.Context, *status.TLStatusGetChannelOnlineUsers) (*status.Vector_Long, error) {
	return nil, nil
}

func (r *recordingStatus) StatusSetUserChannelsOnline(context.Context, *status.TLStatusSetUserChannelsOnline) (*mtproto.Bool, error) {
	return nil, nil
}

func (r *recordingStatus) StatusSetUserChannelsOffline(context.Context, *status.TLStatusSetUserChannelsOffline) (*mtproto.Bool, error) {
	return nil, nil
}

func (r *recordingStatus) StatusSetChannelUserOffline(context.Context, *status.TLStatusSetChannelUserOffline) (*mtproto.Bool, error) {
	return nil, nil
}

func (r *recordingStatus) StatusSetChannelUsersOnline(context.Context, *status.TLStatusSetChannelUsersOnline) (*mtproto.Bool, error) {
	return nil, nil
}

func (r *recordingStatus) StatusSetChannelOffline(context.Context, *status.TLStatusSetChannelOffline) (*mtproto.Bool, error) {
	return nil, nil
}
