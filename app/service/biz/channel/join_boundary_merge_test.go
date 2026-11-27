package channel

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// The paused save is an in-memory interleaving of SaveFeatures. It is not a
// MySQL transaction and not an MTProto session.
func TestPausedFeatureSaveKeepsBothJoinTops(t *testing.T) {
	svc, ctx := newTest(t)
	const (
		owner int64 = 1
		same  int32 = 1_700_000_300
	)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: owner, Title: "Race", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	sent, err := svc.Send(ctx, owner, ch.Id, 0, []Outgoing{{RandomID: 21, Message: &mtproto.Message{
		Message: "race-secret",
		Date:    same,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	secretID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err = svc.TogglePreHistory(ctx, owner, 0, in, true); err != nil {
		t.Fatal(err)
	}
	base := svc.store
	gate := &saveGate{Store: base, started: make(chan struct{}), release: make(chan struct{})}
	svc.store = gate
	gate.arm = true
	errCh := make(chan error, 1)
	go func() {
		_, err := svc.Invite(ctx, owner, 0, in, []int64{21}, nil)
		errCh <- err
	}()
	select {
	case <-gate.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first join did not reach SaveFeatures")
	}
	if _, err = svc.Invite(ctx, owner, 0, in, []int64{22}, nil); err != nil {
		t.Fatal(err)
	}
	close(gate.release)
	if err = <-errCh; err != nil {
		t.Fatal(err)
	}
	svc.store = base
	feat, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if feat.JoinTop[21] < secretID || feat.JoinTop[22] < secretID {
		t.Fatalf("paused saves kept JoinTop=%v secret=%d", feat.JoinTop, secretID)
	}
	if !feat.HiddenPrehistory {
		t.Fatal("merge cleared hidden prehistory")
	}
	for _, user := range []int64{21, 22} {
		stampJoinDate(t, svc, ctx, ch.Id, user, same)
		p, err := svc.store.GetParticipant(ctx, ch.Id, user)
		if err != nil || p == nil || p.State != StateNormal {
			t.Fatalf("user %d membership %+v %v", user, p, err)
		}
		hist, err := svc.History(ctx, user, in, HistoryQuery{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if prehistHas(hist.Messages, "race-secret") {
			t.Fatalf("user %d history returned race-secret with JoinTop %d", user, feat.JoinTop[user])
		}
	}
}

func TestStaleFeatureSaveKeepsUnrelatedEdits(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Merge", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.TogglePreHistory(ctx, 1, 0, inCh(created.Chats[0]), true); err != nil {
		t.Fatal(err)
	}
	a, err := svc.store.GetFeatures(ctx, created.Chats[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.store.GetFeatures(ctx, created.Chats[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	if a.JoinTop == nil {
		a.JoinTop = map[int64]int32{}
	}
	a.JoinTop[21] = 2
	a.AdminLog = append(a.AdminLog, AdminEvent{ID: 91001, Date: 8, UserID: 1})
	b.SlowmodeSeconds = 30
	b.AdminLog = append(b.AdminLog, AdminEvent{ID: 91002, Date: 9, UserID: 1})
	if err = svc.store.SaveFeatures(ctx, created.Chats[0].Id, a); err != nil {
		t.Fatal(err)
	}
	if err = svc.store.SaveFeatures(ctx, created.Chats[0].Id, b); err != nil {
		t.Fatal(err)
	}
	got, err := svc.mutableFeatures(ctx, created.Chats[0].Id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HiddenPrehistory || got.SlowmodeSeconds != 30 || got.JoinTop[21] != 2 {
		t.Fatalf("merged features %+v", got)
	}
	ids := map[int64]bool{}
	for _, ev := range got.AdminLog {
		ids[ev.ID] = true
	}
	if !ids[91001] || !ids[91002] {
		t.Fatalf("admin log lost an entry: %+v", got.AdminLog)
	}
}

func TestReinviteRepairsMissingJoinTopOnlyInsideJoinSecond(t *testing.T) {
	svc, ctx := newTest(t)
	const same int32 = 1_700_000_300
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Residue", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 31, Message: &mtproto.Message{
		Message: "race-secret",
		Date:    same,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	secretID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err = svc.TogglePreHistory(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	feat, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	recorded := feat.JoinTop[21]
	if recorded < secretID {
		t.Fatalf("join top %d does not cover %d", recorded, secretID)
	}
	delete(feat.JoinTop, 21)
	if err = svc.store.SaveFeatures(ctx, ch.Id, feat); err != nil {
		t.Fatal(err)
	}
	st, err := svc.store.GetRead(ctx, ch.Id, 21)
	if err != nil || st == nil || st.ReadInboxMaxID != recorded {
		t.Fatalf("inbox %+v %v", st, err)
	}
	// Pull every message inside the join cursor onto the member's second.
	// A create notice can be newer than the secret while still having a
	// lower id; the repair must see one second, which is the real residue.
	msgs, err := svc.store.ListMessages(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	var joinSecond int32
	for _, m := range msgs {
		if m != nil && !m.Deleted && m.ID <= recorded && m.Date > joinSecond {
			joinSecond = m.Date
		}
	}
	if joinSecond == 0 {
		t.Fatal("join cursor covers no message")
	}
	setMessageDate(t, svc, ch.Id, secretID, joinSecond)
	stampJoinDate(t, svc, ctx, ch.Id, 21, joinSecond)
	hist, err := svc.History(ctx, 21, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !prehistHas(hist.Messages, "race-secret") {
		t.Fatal("missing JoinTop did not show the same-second body before reinvite")
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	feat, err = svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if feat.JoinTop[21] != recorded {
		t.Fatalf("reinvite JoinTop=%d want the inbox snapshot %d", feat.JoinTop[21], recorded)
	}
	hist, err = svc.History(ctx, 21, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(hist.Messages, "race-secret") {
		t.Fatal("reinvite left the same-second body visible")
	}
	later, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 32, Message: &mtproto.Message{
		Message: "after-repair",
		Date:    joinSecond + 100,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	feat, err = svc.mutableFeatures(ctx, ch.Id)
	if err != nil || feat.JoinTop[21] != recorded {
		t.Fatalf("second reinvite moved JoinTop to %d (%v)", feat.JoinTop[21], err)
	}
	hist, err = svc.History(ctx, 21, in, HistoryQuery{Limit: 50})
	if err != nil || !prehistHas(hist.Messages, "after-repair") || prehistHas(hist.Messages, "race-secret") {
		t.Fatalf("history after repair %+v %v", prehistTexts(hist.Messages), err)
	}
	_ = later

	legacy, err := svc.Invite(ctx, 1, 0, in, []int64{22}, nil)
	if err != nil || legacy == nil {
		t.Fatal(err)
	}
	stampJoinDate(t, svc, ctx, ch.Id, 22, 100)
	st, err = svc.store.GetRead(ctx, ch.Id, 22)
	if err != nil || st == nil {
		t.Fatal(err)
	}
	st.ReadInboxMaxID = secretID
	if err = svc.store.SaveRead(ctx, st); err != nil {
		t.Fatal(err)
	}
	feat, err = svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	delete(feat.JoinTop, 22)
	if err = svc.store.SaveFeatures(ctx, ch.Id, feat); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{22}, nil); err != nil {
		t.Fatal(err)
	}
	feat, err = svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if feat.JoinTop[22] != 0 {
		t.Fatalf("reinvite invented JoinTop %d for a cursor past the join second", feat.JoinTop[22])
	}
	hist, err = svc.History(ctx, 22, in, HistoryQuery{Limit: 50})
	if err != nil || !prehistHas(hist.Messages, "after-repair") || !prehistHas(hist.Messages, "race-secret") {
		t.Fatalf("unresolved missing JoinTop changed visibility: %+v %v", prehistTexts(hist.Messages), err)
	}
}

func TestLeaveAndRejoinMovesJoinTop(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Rejoin", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err = svc.TogglePreHistory(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 41, Message: &mtproto.Message{Message: "first", Date: 50}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	first := joinTop(mustFeatures(t, svc, ctx, ch.Id), 21)
	if _, err = svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 42, Message: &mtproto.Message{Message: "second", Date: 60}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Leave(ctx, 21, 0, in); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	second := joinTop(mustFeatures(t, svc, ctx, ch.Id), 21)
	if second <= first {
		t.Fatalf("rejoin JoinTop %d did not move past %d", second, first)
	}
}

func setMessageDate(t *testing.T, svc *Service, channelID int64, msgID, date int32) {
	t.Helper()
	mem, ok := svc.store.(*memoryStore)
	if !ok {
		t.Fatal("expected the memory store")
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	msg := mem.messages[channelID][msgID]
	if msg == nil {
		t.Fatalf("message %d is missing", msgID)
	}
	msg.Date = date
}

func mustFeatures(t *testing.T, svc *Service, ctx context.Context, channelID int64) *Features {
	t.Helper()
	f, err := svc.mutableFeatures(ctx, channelID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// saveGate stalls the first join-boundary write before the store records it
// and lets the next SaveFeatures finish. Memory only.
type saveGate struct {
	Store
	mu      sync.Mutex
	arm     bool
	held    bool
	started chan struct{}
	release chan struct{}
}

func (g *saveGate) SaveFeatures(ctx context.Context, channelID int64, f *Features) error {
	g.mu.Lock()
	hold := g.arm && !g.held && f != nil && len(f.JoinTop) > 0
	if hold {
		g.held = true
	}
	g.mu.Unlock()
	if hold {
		close(g.started)
		<-g.release
	}
	return g.Store.SaveFeatures(ctx, channelID, f)
}
