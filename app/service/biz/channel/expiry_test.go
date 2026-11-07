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
