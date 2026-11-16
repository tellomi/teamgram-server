package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// Synthetic users only. Memory store: no MySQL, no Kafka, no phone session.
const (
	boundOwner int64 = 910177001
	boundNew   int64 = 910177002
)

// TestPrehistoryBoundary covers three gaps left after 7ada2737:
// a message from the same second as join but already stored, a failed
// member-row read, and an admin who can read an old message without having
// read it. One process, in memory.
func TestPrehistoryBoundary(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: boundOwner, Title: "Boundary", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	const same int32 = 1_700_000_000
	sent, err := svc.Send(ctx, boundOwner, ch.Id, 0, []Outgoing{{RandomID: 910177011, Message: &mtproto.Message{
		Message: "same-second-secret",
		Date:    same,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	oldID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err = svc.PinMessage(ctx, boundOwner, 0, ch.Id, oldID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.TogglePreHistory(ctx, boundOwner, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, boundOwner, 0, in, []int64{boundNew}, nil); err != nil {
		t.Fatal(err)
	}
	topAtJoin := joinTop(svc.features(ctx, ch.Id), boundNew)
	if topAtJoin < oldID {
		t.Fatalf("join top %d does not cover the pre-join message %d", topAtJoin, oldID)
	}
	newbie, err := svc.store.GetParticipant(ctx, ch.Id, boundNew)
	if err != nil || newbie == nil {
		t.Fatal(err)
	}
	newbie.Date = same
	if err = svc.store.PutParticipant(ctx, newbie); err != nil {
		t.Fatal(err)
	}
	fresh, err := svc.Send(ctx, boundOwner, ch.Id, 0, []Outgoing{{RandomID: 910177012, Message: &mtproto.Message{
		Message: "same-second-open",
		Date:    same,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	freshID := fresh.Updates[0].GetMessage_MESSAGE().GetId()
	if freshID <= topAtJoin {
		t.Fatalf("post-join id %d is not past join top %d", freshID, topAtJoin)
	}

	hist, err := svc.History(ctx, boundNew, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(hist.Messages, "same-second-secret") || !prehistHas(hist.Messages, "same-second-open") {
		t.Fatalf("getHistory texts %v", prehistTexts(hist.Messages))
	}
	if hist.Count <= int32(len(prehistTexts(hist.Messages))) {
		t.Fatalf("history count %d is not the unfiltered residual", hist.Count)
	}
	got, err := svc.GetMessages(ctx, boundNew, in, []int32{oldID, freshID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty || got.Messages[1].GetMessage() != "same-second-open" {
		t.Fatalf("getMessages %+v", prehistTexts(got.Messages))
	}
	diff, err := svc.Difference(ctx, boundNew, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(diff.NewMessages, "same-second-secret") || updateHas(diff.OtherUpdates, "same-second-secret") {
		t.Fatalf("difference leaked %v", prehistTexts(diff.NewMessages))
	}
	if !prehistHas(diff.NewMessages, "same-second-open") && !updateHas(diff.OtherUpdates, "same-second-open") {
		t.Fatalf("difference dropped the post-join message %v", prehistTexts(diff.NewMessages))
	}
	if diff.Pts <= 0 {
		t.Fatal("difference pts is the residual cursor and should stay")
	}
	dropChannelPts(t, svc, ch.Id, func(*PtsEvent) bool { return true })
	longDiff, err := svc.Difference(ctx, boundNew, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(longDiff.Messages, "same-second-secret") || prehistHas(longDiff.NewMessages, "same-second-secret") {
		t.Fatalf("tooLong leaked %s %v", longDiff.GetPredicateName(), prehistTexts(append(longDiff.Messages, longDiff.NewMessages...)))
	}
	dlg := longDiff.GetDialog()
	if dlg.GetPts().GetValue() == 0 || dlg.GetReadInboxMaxId() == 0 || dlg.GetTopMessage() == 0 {
		t.Fatalf("tooLong dialog lost its id residual %+v", dlg)
	}
	if svc.features(ctx, ch.Id).PinnedMsgID != oldID {
		t.Fatalf("pinned id residual = %d", svc.features(ctx, ch.Id).PinnedMsgID)
	}

	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, boundOwner, 0, in, boundNew, rights, ""); err != nil {
		t.Fatal(err)
	}
	asAdmin, err := svc.GetMessages(ctx, boundNew, in, []int32{oldID})
	if err != nil || asAdmin.Messages[0].GetMessage() != "same-second-secret" {
		t.Fatalf("admin should be allowed to read the old body: %+v %v", asAdmin, err)
	}
	readers, err := svc.ReadParticipants(ctx, boundOwner, ch.Id, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if readerHas(readers, boundNew) {
		t.Fatalf("promotion listed the admin before readHistory: %+v", readers)
	}
	if err = svc.ReadHistory(ctx, boundNew, in, freshID); err != nil {
		t.Fatal(err)
	}
	readers, err = svc.ReadParticipants(ctx, boundOwner, ch.Id, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if !readerHas(readers, boundNew) {
		t.Fatalf("readHistory did not count the admin: %+v", readers)
	}

	base := svc.store
	svc.store = &failNthParticipant{Store: base, user: boundNew}
	hidden, err := svc.GetMessages(ctx, boundNew, in, []int32{oldID})
	svc.store = base
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Messages[0].GetMessage() == "same-second-secret" || hidden.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty {
		t.Fatalf("member read error returned the body: %+v", hidden.Messages[0])
	}
	if !prehistoryBlocks(&Features{HiddenPrehistory: true}, nil, &StoredMessage{ID: 1, Date: same}, errors.New("read failed")) {
		t.Fatal("load error fail-open")
	}
	if prehistoryBlocks(&Features{HiddenPrehistory: true}, nil, &StoredMessage{ID: 1, Date: same}, nil) {
		t.Fatal("a nil member with no error should stay unchanged")
	}
}

type failNthParticipant struct {
	Store
	user int64
	n    int
}

func (f *failNthParticipant) GetParticipant(ctx context.Context, channelID, userID int64) (*Participant, error) {
	if userID == f.user {
		f.n++
		if f.n >= 2 {
			return nil, errors.New("participant read failed")
		}
	}
	return f.Store.GetParticipant(ctx, channelID, userID)
}
