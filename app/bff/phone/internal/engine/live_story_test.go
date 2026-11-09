package engine

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
)

func newLiveEngine(t *testing.T) (*Engine, *func()) {
	t.Helper()
	var fire func()
	eng := New(Options{
		Config: config.Config{}.Normalized(),
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		After: func(_ time.Duration, fn func()) func() {
			fire = fn
			return func() {}
		},
	})
	return eng, &fire
}

func TestLiveDiscardEndsImmediately(t *testing.T) {
	eng, fire := newLiveEngine(t)
	var reasons []string
	eng.SetOnLive(func(ev LiveEnd) { reasons = append(reasons, ev.Reason) })
	var viewers []int64
	eng.push = func(n Note) { viewers = append(viewers, n.UserID) }
	id, access, up, err := eng.CreateLiveStory(CreateLiveStory{Host: 4, Auth: 8, Random: 1, Audience: []int64{5}})
	if err != nil || id == 0 || access == 0 {
		t.Fatal(err, id, access)
	}
	var live bool
	for _, u := range up.GetUpdates() {
		if u.GetLiveStory() && u.GetCall_GROUPCALL().GetId() == id {
			live = true
		}
	}
	if !live {
		t.Fatalf("start updates %+v", up)
	}
	if !containsInt(viewers, 4) || !containsInt(viewers, 5) {
		t.Fatalf("audience %v", viewers)
	}
	discarded, err := eng.DiscardGroup(4, id, access, true)
	if err != nil {
		t.Fatal(err)
	}
	var gone bool
	for _, u := range discarded.GetUpdates() {
		if u.GetLiveStory() && u.GetCall_GROUPCALL().GetPredicateName() == mtproto.Predicate_groupCallDiscarded {
			gone = true
		}
	}
	if !gone || len(reasons) != 1 || reasons[0] != "discard" {
		t.Fatalf("discard reasons=%v updates=%+v", reasons, discarded)
	}
	(*fire)()
	if len(reasons) != 1 {
		t.Fatalf("join timer ended a discarded call %v", reasons)
	}
}

func TestLiveLeaveAndOfflineTimeout(t *testing.T) {
	eng, fire := newLiveEngine(t)
	var reasons []string
	eng.SetOnLive(func(ev LiveEnd) { reasons = append(reasons, ev.Reason) })
	id, access, _, err := eng.CreateLiveStory(CreateLiveStory{Host: 4, Auth: 8, Random: 2})
	if err != nil {
		t.Fatal(err)
	}
	offer := `{"ssrc":222,"ufrag":"cu","pwd":"cp","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB","setup":"passive"}]}`
	if _, err = eng.JoinGroup(JoinGroup{
		UserID: 4, Auth: 8, CallID: id, Access: access, Params: offer,
		PeerType: mtproto.PEER_USER, PeerID: 4,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = eng.LeaveGroup(4, id, access, 0); err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 {
		t.Fatalf("leave ended immediately %v", reasons)
	}
	(*fire)()
	if len(reasons) != 1 || reasons[0] != "gone" {
		t.Fatalf("leave timeout %v", reasons)
	}
}

func TestLiveHostOfflineAndJoinTimeout(t *testing.T) {
	eng, fire := newLiveEngine(t)
	var reasons []string
	eng.SetOnLive(func(ev LiveEnd) { reasons = append(reasons, ev.Reason) })
	if _, _, _, err := eng.CreateLiveStory(CreateLiveStory{Host: 4, Auth: 8, Random: 3}); err != nil {
		t.Fatal(err)
	}
	(*fire)()
	if len(reasons) != 1 || reasons[0] != "join" {
		t.Fatalf("join timeout %v", reasons)
	}

	eng, fire = newLiveEngine(t)
	reasons = nil
	eng.SetOnLive(func(ev LiveEnd) { reasons = append(reasons, ev.Reason) })
	if _, _, _, err := eng.CreateLiveStory(CreateLiveStory{Host: 6, Auth: 1, Random: 4}); err != nil {
		t.Fatal(err)
	}
	eng.NoteHostOffline(6, 1)
	(*fire)()
	if len(reasons) != 1 || reasons[0] != "gone" {
		t.Fatalf("offline timeout %v", reasons)
	}
}

func containsInt(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
