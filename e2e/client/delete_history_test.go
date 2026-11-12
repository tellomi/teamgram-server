package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestDeleteHistoryByDateAndOtherDevice deletes one slice of a private chat
// by min_date/max_date. The messages outside that range stay. A second
// session of the same account sees updateDeleteMessages and does not load
// the deleted text back from getHistory or getDialogs.
func TestDeleteHistoryByDateAndOtherDevice(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "delhist_a", "DelA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "delhist_b", "DelB")
	a2, _ := loginNamed(t, cfg, ProfileAndroid, "delhist_a", "DelA")
	aToB := userPeer(t, cfg, a, "delhist_b", "DelB")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	early := fmt.Sprintf("delhist-early-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, aToB, early, nil); err != nil {
		t.Fatal(err)
	}
	if err := waitUntilNextSecond(ctx); err != nil {
		t.Fatal(err)
	}
	mid := fmt.Sprintf("delhist-mid-%d", time.Now().UnixNano())
	midID, err := a.SendText(ctx, aToB, mid, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitUntilNextSecond(ctx); err != nil {
		t.Fatal(err)
	}
	late := fmt.Sprintf("delhist-late-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, aToB, late, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := a.History(ctx, aToB, 20)
	if err != nil {
		t.Fatal(err)
	}
	earlyDate, midDate, lateDate := dateOfText(sent, early), dateOfText(sent, mid), dateOfText(sent, late)
	if earlyDate == 0 || midDate == 0 || lateDate == 0 || earlyDate >= midDate || lateDate <= midDate {
		t.Fatalf("message dates early=%d mid=%d late=%d", earlyDate, midDate, lateDate)
	}
	minDate, maxDate := midDate, midDate
	pts, qts, date, err := a2.State(ctx)
	if err != nil {
		t.Fatal(err)
	}

	affected := deleteHistory(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        aToB,
		Revoke:      true,
		MaxId:       midID,
		MinDate:     &wrapperspb.Int32Value{Value: minDate},
		MaxDate:     &wrapperspb.Int32Value{Value: maxDate},
	})
	if affected.GetPtsCount() < 1 {
		t.Fatalf("date delete reported success without removing anything: %#v", affected)
	}
	for affected.GetOffset() > 0 {
		affected = deleteHistory(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
			Constructor: mtproto.CRC32_messages_deleteHistory,
			Peer:        aToB,
			Revoke:      true,
			MaxId:       midID,
			MinDate:     &wrapperspb.Int32Value{Value: minDate},
			MaxDate:     &wrapperspb.Int32Value{Value: maxDate},
		})
	}

	assertHistoryTexts(t, ctx, a, aToB, map[string]bool{early: true, late: true, mid: false})
	assertHistoryTexts(t, ctx, a2, aToB, map[string]bool{early: true, late: true, mid: false})
	bToA := userPeer(t, cfg, b, "delhist_a", "DelA")
	deadline := time.Now().Add(20 * time.Second)
	for {
		msgs, err := b.History(ctx, bToA, 40)
		if err != nil {
			t.Fatal(err)
		}
		if idOfText(msgs, mid) == 0 && idOfText(msgs, early) != 0 && idOfText(msgs, late) != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer history did not drop only the dated message; still seeing mid=%d", idOfText(msgs, mid))
		}
		time.Sleep(300 * time.Millisecond)
	}
	if dialogShowsText(t, ctx, a2, bSelf.ID, mid) {
		t.Fatal("getDialogs still shows the deleted message")
	}
	diff, err := a2.Difference(ctx, pts, qts, date)
	if err != nil {
		t.Fatal(err)
	}
	if !differenceDeletes(diff, midID) {
		t.Fatalf("other device difference missing updateDeleteMessages: %s", summarizeUpdates(diff))
	}
}

// TestDeleteHistoryBatchOffsetContinues sends more messages than one slice
// and checks the first reply asks the client to call again. A forced failure
// is covered in the in-process test: the slice that fails is not deleted,
// and the next call continues. This test is that continuation against the
// server, including the messages that remain visible until the last slice.
func TestDeleteHistoryBatchOffsetContinues(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "delbatch_a", "BatchA")
	_, _ = loginNamed(t, cfg, ProfileAndroid, "delbatch_b", "BatchB")
	aToB := userPeer(t, cfg, a, "delbatch_b", "BatchB")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const n = 101
	prefix := fmt.Sprintf("delbatch-%d-", time.Now().UnixNano())
	oldest := prefix + "0"
	newest := prefix + fmt.Sprint(n-1)
	for i := 0; i < n; i++ {
		if _, err := a.SendText(ctx, aToB, prefix+fmt.Sprint(i), nil); err != nil {
			t.Fatal(err)
		}
	}
	first := deleteHistory(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        aToB,
	})
	if first.GetOffset() <= 0 || first.GetPtsCount() <= 0 {
		t.Fatalf("first slice did not ask the client to continue: %#v", first)
	}
	left, err := a.History(ctx, aToB, 20)
	if err != nil {
		t.Fatal(err)
	}
	if idOfText(left, newest) != 0 {
		t.Fatal("newest message survived the first slice")
	}
	if idOfText(left, oldest) == 0 {
		t.Fatal("oldest message was deleted before the client continued")
	}

	affected := first
	for affected.GetOffset() > 0 {
		affected = deleteHistory(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
			Constructor: mtproto.CRC32_messages_deleteHistory,
			Peer:        aToB,
		})
	}
	left, err = a.History(ctx, aToB, 40)
	if err != nil {
		t.Fatal(err)
	}
	if idOfText(left, oldest) != 0 || idOfText(left, newest) != 0 {
		t.Fatal("history still returns messages after offset reached 0")
	}
}

func deleteHistory(t *testing.T, ctx context.Context, c *Client, req *mtproto.TLMessagesDeleteHistory) *mtproto.Messages_AffectedHistory {
	t.Helper()
	obj, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesAffectedHistory:
		return v.To_Messages_AffectedHistory()
	case *mtproto.Messages_AffectedHistory:
		return v
	default:
		t.Fatalf("messages.deleteHistory returned %T", obj)
		return nil
	}
}

func dateOfText(msgs []*mtproto.Message, text string) int32 {
	for _, msg := range msgs {
		if msg.GetMessage() == text {
			return msg.GetDate()
		}
	}
	return 0
}

func assertHistoryTexts(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, want map[string]bool) {
	t.Helper()
	msgs, err := c.History(ctx, peer, 40)
	if err != nil {
		t.Fatal(err)
	}
	for text, keep := range want {
		got := idOfText(msgs, text) != 0
		if got != keep {
			t.Fatalf("history text %q present=%v want %v", text, got, keep)
		}
	}
}

func dialogShowsText(t *testing.T, ctx context.Context, c *Client, peerID int64, text string) bool {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var dialogs []*mtproto.Dialog
	var messages []*mtproto.Message
	switch v := obj.(type) {
	case *mtproto.TLMessagesDialogs:
		dialogs, messages = v.GetDialogs(), v.GetMessages()
	case *mtproto.TLMessagesDialogsSlice:
		dialogs, messages = v.GetDialogs(), v.GetMessages()
	default:
		t.Fatalf("getDialogs %T", obj)
	}
	var top int32
	for _, d := range dialogs {
		if d.GetPeer().GetUserId() == peerID {
			top = d.GetTopMessage()
		}
	}
	if top == 0 {
		return false
	}
	for _, m := range messages {
		if m.GetId() == top && m.GetMessage() == text {
			return true
		}
	}
	return false
}
