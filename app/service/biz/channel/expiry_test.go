package channel

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/msgexpiry"
)

func TestExpiredMessagesStayUntilTheSwitch(t *testing.T) {
	t.Setenv(msgexpiry.EnvEnabled, "")
	t.Setenv(msgexpiry.EnvSweep, "")
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Expire")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 41,
		Message:  &mtproto.Message{Message: "old", Date: 100},
	}})
	if err != nil {
		t.Fatal(err)
	}
	oldID := messageID(sent)
	if _, err := svc.SetHistoryTTL(ctx, 1, 0, ch.Id, 60); err != nil {
		t.Fatal(err)
	}
	kept, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 42,
		Message:  &mtproto.Message{Message: "keep", Date: int32(time.Now().Unix())},
	}})
	if err != nil {
		t.Fatal(err)
	}
	keepID := messageID(kept)
	if !historyHas(t, svc, ctx, 1, in, "old") || !historyHas(t, svc, ctx, 2, in, "old") {
		t.Fatal("flag off hid a message")
	}

	t.Setenv(msgexpiry.EnvEnabled, "1")
	if historyHas(t, svc, ctx, 1, in, "old") || historyHas(t, svc, ctx, 2, in, "old") {
		t.Fatal("expired text still readable")
	}
	if !historyHas(t, svc, ctx, 1, in, "keep") || !historyHas(t, svc, ctx, 2, in, "keep") {
		t.Fatal("live message hidden")
	}
	got, err := svc.GetMessages(ctx, 2, in, []int32{oldID, keepID})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got.GetMessages() {
		if m.GetId() == oldID && m.GetMessage() != "" {
			t.Fatalf("getMessages returned %q", m.GetMessage())
		}
		if m.GetId() == keepID && m.GetMessage() != "keep" {
			t.Fatalf("keep via id %#v", m)
		}
	}
	if _, err := svc.History(ctx, 99, in, HistoryQuery{Limit: 20}); err == nil {
		t.Fatal("non-member read history")
	}

	var deletes int
	svc.SetHooks(Hooks{Push: func(_ context.Context, _ int64, _ []int64, updates *mtproto.Updates) {
		if findUpdate(updates, mtproto.Predicate_updateDeleteChannelMessages) != nil {
			deletes++
		}
	}})
	t.Setenv(msgexpiry.EnvSweep, "1")
	if _, err := svc.History(ctx, 1, in, HistoryQuery{Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if deletes == 0 {
		t.Fatal("sweep did not tell members")
	}
	first := deletes
	if _, err := svc.History(ctx, 2, in, HistoryQuery{Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if deletes != first {
		t.Fatalf("second sweep pushed again: %d -> %d", first, deletes)
	}
	if historyHas(t, svc, ctx, 1, in, "old") {
		t.Fatal("swept message came back")
	}
}

func TestExpireDueDeletesBothSidesAndIsIdempotent(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "expire")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetHistoryTTL(ctx, 1, 0, ch.Id, 30); err != nil {
		t.Fatal(err)
	}
	now := int32(time.Now().Unix())
	if _, _, err := svc.appendMessage(ctx, ch.Id, 1, 11, mtproto.MakeTLMessage(&mtproto.Message{
		Date:    now - 120,
		Message: "gone",
		FromId:  mtproto.MakePeerUser(1),
	}).To_Message()); err != nil {
		t.Fatal(err)
	}
	keptID, _, err := svc.appendMessage(ctx, ch.Id, 1, 12, mtproto.MakeTLMessage(&mtproto.Message{
		Date:    now,
		Message: "stay",
		FromId:  mtproto.MakePeerUser(1),
	}).To_Message())
	if err != nil {
		t.Fatal(err)
	}

	var deletes []int32
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
		if userID != 2 || updates == nil {
			return
		}
		for _, up := range updates.GetUpdates() {
			if up.GetPredicateName() == mtproto.Predicate_updateDeleteChannelMessages && up.GetPts_INT32() > 0 && up.GetPtsCount() > 0 {
				deletes = append(deletes, up.GetMessages()...)
			}
		}
	}})
	n, err := svc.ExpireDue(ctx, now, 50)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(deletes) != 1 {
		t.Fatalf("deleted %d ids %#v, want the one expired post", n, deletes)
	}
	if !historyHas(t, svc, ctx, 2, in, "stay") || historyHas(t, svc, ctx, 2, in, "gone") {
		t.Fatal("expired channel post still in history, or the live one was removed")
	}
	hist, err := svc.History(ctx, 2, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range hist.GetMessages() {
		if msg.GetId() == keptID && msg.GetTtlPeriod().GetValue() != 30 {
			t.Fatalf("stamped ttl = %d", msg.GetTtlPeriod().GetValue())
		}
	}
	again, err := svc.ExpireDue(ctx, now, 50)
	if err != nil || again != 0 {
		t.Fatalf("second pass deleted %d (%v), want 0", again, err)
	}
}

func historyHas(t *testing.T, svc *Service, ctx context.Context, user int64, in *mtproto.InputChannel, text string) bool {
	t.Helper()
	hist, err := svc.History(ctx, user, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hist.GetMessages() {
		if m.GetMessage() == text {
			return true
		}
	}
	return false
}
