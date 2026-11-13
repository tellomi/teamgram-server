package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestDeleteHistoryMaxIDLastClearsPeer is ISSUE-S-DELHIST-MAXID-001.
// A calls messages.deleteHistory(peer=B, just_clear, revoke, max_id=A's last
// box id). Official Telegram clears both sides: B's own last message is
// removed and B's dialog top becomes messageActionHistoryClear.
// max_id is A's box id. B's box ids are a different sequence. The test
// pushes B's sequence ahead first, so a comparison with A's number leaves
// B's last message in place.
func TestDeleteHistoryMaxIDLastClearsPeer(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "delhist_max_a", "DelHistA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "delhist_max_b", "DelHistB")
	c, _ := loginNamed(t, cfg, ProfileIOS, "delhist_max_c", "DelHistC")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	aToB := userPeer(t, cfg, a, "delhist_max_b", "DelHistB")
	bToA := userPeer(t, cfg, b, "delhist_max_a", "DelHistA")
	bToC := userPeer(t, cfg, b, "delhist_max_c", "DelHistC")
	_ = c

	var (
		fromA    string
		fromB    string
		idOnA    int32
		idOnB    int32
		diverged bool
	)
	for round := 0; round < 6 && !diverged; round++ {
		for i := 0; i < 20; i++ {
			if _, err := b.SendText(ctx, bToC, fmt.Sprintf("delhist-pad-%d-%d", round, i), nil); err != nil {
				t.Fatal(err)
			}
		}
		fromA = fmt.Sprintf("delhist-from-a-%d", time.Now().UnixNano())
		fromB = fmt.Sprintf("delhist-from-b-%d", time.Now().UnixNano())
		if _, err := a.SendText(ctx, aToB, fromA, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := b.SendText(ctx, bToA, fromB, nil); err != nil {
			t.Fatal(err)
		}
		if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
			aMsgs, err := a.History(ctx, aToB, 30)
			if err != nil {
				return false, err
			}
			bMsgs, err := b.History(ctx, bToA, 30)
			if err != nil {
				return false, err
			}
			idOnA = idOfText(aMsgs, fromB)
			idOnB = idOfText(bMsgs, fromB)
			return idOnA != 0 && idOnB != 0 && idOfText(bMsgs, fromA) != 0, nil
		}); err != nil {
			t.Fatal(err)
		}
		diverged = idOnB > idOnA
	}
	if !diverged {
		t.Fatalf("box ids did not diverge: A last %d, B last %d", idOnA, idOnB)
	}

	ptsB, qtsB, dateB, err := b.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cleared := deleteHistoryUntil(t, ctx, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        aToB,
		JustClear:   true,
		Revoke:      true,
		MaxId:       idOnA,
	})
	if cleared.GetOffset() != 0 {
		t.Fatalf("deleteHistory offset %d", cleared.GetOffset())
	}

	deadline := time.Now().Add(30 * time.Second)
	var lastB mtproto.TLObject
	var histA, histB []*mtproto.Message
	for {
		histA, err = a.History(ctx, aToB, 40)
		if err != nil {
			t.Fatal(err)
		}
		histB, err = b.History(ctx, bToA, 40)
		if err != nil {
			t.Fatal(err)
		}
		lastB, err = b.Difference(ctx, ptsB, qtsB, dateB)
		if err != nil {
			t.Fatal(err)
		}
		topA := userDialogTop(t, ctx, a, aToB.GetUserId())
		topB := userDialogTop(t, ctx, b, bToA.GetUserId())
		if idOfText(histA, fromA) == 0 && idOfText(histA, fromB) == 0 &&
			idOfText(histB, fromA) == 0 && idOfText(histB, fromB) == 0 &&
			isHistoryClear(topA) && isHistoryClear(topB) &&
			differenceEditsHistoryClear(lastB, idOnB) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("A max_id=%d (B box %d) left B's last message: A top %s B top %s B diff %s",
				idOnA, idOnB, describeMessage(topA), describeMessage(topB), summarizeUpdates(lastB))
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(400 * time.Millisecond):
		}
	}
}

func userDialogTop(t *testing.T, ctx context.Context, c *Client, peerID int64) *mtproto.Message {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var dialogs []*mtproto.Dialog
	var messages []*mtproto.Message
	switch d := res.(type) {
	case *mtproto.TLMessagesDialogs:
		dialogs, messages = d.GetDialogs(), d.GetMessages()
	case *mtproto.TLMessagesDialogsSlice:
		dialogs, messages = d.GetDialogs(), d.GetMessages()
	default:
		return nil
	}
	var topID int32
	for _, d := range dialogs {
		if d.GetPeer().GetUserId() == peerID {
			topID = d.GetTopMessage()
			break
		}
	}
	if topID == 0 {
		return nil
	}
	for _, m := range messages {
		if m.GetId() == topID {
			return m
		}
	}
	return nil
}

func isHistoryClear(m *mtproto.Message) bool {
	return m != nil && m.GetAction().GetPredicateName() == mtproto.Predicate_messageActionHistoryClear
}

func describeMessage(m *mtproto.Message) string {
	if m == nil {
		return "none"
	}
	if isHistoryClear(m) {
		return fmt.Sprintf("historyClear id=%d", m.GetId())
	}
	return fmt.Sprintf("%s id=%d text=%q", m.GetPredicateName(), m.GetId(), m.GetMessage())
}

func differenceEditsHistoryClear(obj mtproto.TLObject, messageID int32) bool {
	for _, up := range differenceUpdates(obj) {
		if up.GetPredicateName() != mtproto.Predicate_updateEditMessage || up.GetPts_INT32() == 0 {
			continue
		}
		m := up.GetMessage_MESSAGE()
		if m.GetId() == messageID && isHistoryClear(m) {
			return true
		}
	}
	return false
}
