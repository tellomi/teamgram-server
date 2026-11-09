package sess

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/status/status"
)

// A second connection on the same auth key stays up while the updates
// session's socket closes. The user must stay online and the push target
// must stay in place, or the next message is never pushed live.
func TestOfflineWaitsForEveryConnection(t *testing.T) {
	rec := &recordingStatus{}
	m := newTestWrapper(rec)

	updates := onlineSession(m, m.mainAuth, 1)
	updates.isGeneric = true
	m.mainUpdatesSession = updates
	other := onlineSession(m, m.tempAuth, 2)

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

	other.isGeneric = true
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

func TestUpdatesGoToEveryOnlineSession(t *testing.T) {
	m := newTestWrapper(nil)
	first := onlineSession(m, m.mainAuth, 1)
	first.isGeneric = true
	first.canSync = true
	m.mainUpdatesSession = first
	second := onlineSession(m, m.tempAuth, 2)
	second.canSync = true
	closed := onlineSession(m, m.mediaTempAuth, 3)
	closed.changeConnState(context.Background(), kStateOffline)

	m.onSyncData(context.Background(), &syncData{
		data: &messageData{obj: mtproto.MakeTLUpdatesTooLong(nil)},
	})

	if first.outQueue.oMsgs.Len() == 0 {
		t.Fatal("updates session received nothing")
	}
	if second.outQueue.oMsgs.Len() == 0 {
		t.Fatal("second connection received nothing")
	}
	if closed.outQueue.oMsgs.Len() != 0 {
		t.Fatal("closed connection was still a push target")
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
	offline []int64
}

func (r *recordingStatus) StatusSetSessionOnline(context.Context, *status.TLStatusSetSessionOnline) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func (r *recordingStatus) StatusSetSessionOffline(_ context.Context, in *status.TLStatusSetSessionOffline) (*mtproto.Bool, error) {
	if in != nil {
		r.offline = append(r.offline, in.GetAuthKeyId())
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
