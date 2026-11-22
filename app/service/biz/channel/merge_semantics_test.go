package channel

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCloneKeepsBaseSoZeroClearsSlowmode(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Zero")
	f, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	f.SlowmodeSeconds = 40
	if err = svc.store.SaveFeatures(ctx, ch.Id, f); err != nil {
		t.Fatal(err)
	}
	got, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	cp := got.clone()
	if cp.base == nil || cp.base.SlowmodeSeconds != 40 {
		t.Fatalf("clone dropped the read baseline: base=%+v", cp.base)
	}
	cp.SlowmodeSeconds = 0
	if err = svc.store.SaveFeatures(ctx, ch.Id, cp); err != nil {
		t.Fatal(err)
	}
	got, err = svc.mutableFeatures(ctx, ch.Id)
	if err != nil || got.SlowmodeSeconds != 0 {
		t.Fatalf("explicit zero did not clear slowmode: %+v %v", got, err)
	}
}

func TestReorderUsernamesPersistsOrderAndPrimary(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Names")
	in := inCh(ch)
	f, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	f.Usernames = []UsernameRec{
		{Username: "alphaone", Active: true, Editable: true},
		{Username: "betatwo", Active: true, Editable: false},
	}
	f.JoinTop = map[int64]int32{21: 4}
	if err = svc.store.SaveFeatures(ctx, ch.Id, f); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.store.GetChannel(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	stored.Username = "alphaone"
	if err = svc.store.SaveChannel(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ReorderUsernames(ctx, 1, 0, in, []string{"betatwo", "alphaone"}); err != nil {
		t.Fatal(err)
	}
	stored, err = svc.store.GetChannel(ctx, ch.Id)
	if err != nil || stored.Username != "betatwo" {
		t.Fatalf("channel.username=%q %v", stored.Username, err)
	}
	got, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil || len(got.Usernames) != 2 || got.Usernames[0].Username != "betatwo" || got.Usernames[1].Username != "alphaone" {
		t.Fatalf("feature order %+v %v", got.Usernames, err)
	}
	if got.JoinTop[21] != 4 {
		t.Fatalf("reorder dropped JoinTop %+v", got.JoinTop)
	}
}

func TestExplicitDeletesKeepConcurrentKeys(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Delete")
	f, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	f.JoinTop = map[int64]int32{21: 2}
	f.Usernames = []UsernameRec{{Username: "alphaone", Active: true, Editable: true}}
	if err = svc.store.SaveFeatures(ctx, ch.Id, f); err != nil {
		t.Fatal(err)
	}
	clearNames, err := svc.store.GetFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	addOther, err := svc.store.GetFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	clearNames.Usernames = []UsernameRec{}
	delete(clearNames.JoinTop, 21)
	if addOther.JoinTop == nil {
		addOther.JoinTop = map[int64]int32{}
	}
	addOther.JoinTop[22] = 3
	if err = svc.store.SaveFeatures(ctx, ch.Id, addOther); err != nil {
		t.Fatal(err)
	}
	if err = svc.store.SaveFeatures(ctx, ch.Id, clearNames); err != nil {
		t.Fatal(err)
	}
	got, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Usernames) != 0 || got.JoinTop[21] != 0 || got.JoinTop[22] != 3 {
		t.Fatalf("explicit clear lost a concurrent key or kept a deleted one: %+v %+v", got.Usernames, got.JoinTop)
	}
}

func TestStaleLeaveDoesNotUndoRejoin(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "RejoinRace")
	in := inCh(ch)
	if _, err := svc.TogglePreHistory(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 81, Message: &mtproto.Message{Message: "before", Date: 10}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	stale, err := svc.store.GetParticipant(ctx, ch.Id, 21)
	if err != nil || stale == nil || !stale.hasBase {
		t.Fatalf("participant read did not keep a baseline: %+v %v", stale, err)
	}
	first := joinTop(mustFeatures(t, svc, ctx, ch.Id), 21)
	if _, err = svc.Leave(ctx, 21, 0, in); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 82, Message: &mtproto.Message{Message: "after", Date: 20}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	second := joinTop(mustFeatures(t, svc, ctx, ch.Id), 21)
	if second <= first {
		t.Fatalf("rejoin top %d did not move past %d", second, first)
	}
	stale.State = StateLeft
	err = svc.store.PutParticipant(ctx, stale)
	if !errors.Is(err, errParticipantChanged) {
		t.Fatalf("stale leave err=%v", err)
	}
	cur, err := svc.store.GetParticipant(ctx, ch.Id, 21)
	if err != nil || cur == nil || cur.State != StateNormal {
		t.Fatalf("stale leave changed membership %+v %v", cur, err)
	}
	if joinTop(mustFeatures(t, svc, ctx, ch.Id), 21) != second {
		t.Fatal("stale leave rewrote the rejoin boundary")
	}
}

func TestMissingJoinTopPastJoinSecondIsNotGuessed(t *testing.T) {
	svc, ctx := newTest(t)
	const same int32 = 1_700_000_400
	ch := megagroup(t, svc, ctx, "Unresolved")
	in := inCh(ch)
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 91, Message: &mtproto.Message{Message: "race-secret", Date: same}}})
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
	later, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 92, Message: &mtproto.Message{Message: "after-join", Date: same + 100}}})
	if err != nil {
		t.Fatal(err)
	}
	laterID := later.Updates[0].GetMessage_MESSAGE().GetId()
	feat, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	delete(feat.JoinTop, 21)
	if err = svc.store.SaveFeatures(ctx, ch.Id, feat); err != nil {
		t.Fatal(err)
	}
	stampJoinDate(t, svc, ctx, ch.Id, 21, same)
	setMessageDate(t, svc, ch.Id, secretID, same)
	st, err := svc.store.GetRead(ctx, ch.Id, 21)
	if err != nil || st == nil {
		t.Fatal(err)
	}
	st.ReadInboxMaxID = laterID
	if err = svc.store.SaveRead(ctx, st); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{21}, nil); err != nil {
		t.Fatal(err)
	}
	feat, err = svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if feat.JoinTop[21] != 0 {
		t.Fatalf("guessed JoinTop %d without the id captured at join", feat.JoinTop[21])
	}
	hist, err := svc.History(ctx, 21, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	// The same-second body stays visible. Hiding it would require the lost
	// join id. The later message must stay visible either way.
	if !prehistHas(hist.Messages, "race-secret") || !prehistHas(hist.Messages, "after-join") {
		t.Fatalf("visibility %+v", prehistTexts(hist.Messages))
	}
}
