package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestHistoryClearReachesOtherSession is 行 134 / ISSUE-S-DELHIST-SYNC-001.
// just_clear rewrites the newest message to messageActionHistoryClear and
// must also name that id in updateDeleteMessages. The other session of the
// same account otherwise keeps the last bubble until getHistory.
// Phones 9996610651 / 9996610652, code 11111.
func TestHistoryClearReachesOtherSession(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610651", "ClearA")
	a2, _ := dialPhone(t, ctx, cfg, "9996610651", "ClearA")
	b, _ := dialPhone(t, ctx, cfg, "9996610652", "ClearB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610652", "ClearB")
	if err != nil {
		t.Fatal(err)
	}
	_ = b
	peer := inputPeerUser(bID, bHash)

	var lastID int32
	for i := 0; i < 3; i++ {
		id, err := a.SendText(ctx, peer, fmt.Sprintf("clear-sync-%d-%d", i, time.Now().UnixNano()), nil)
		if err != nil {
			t.Fatal(err)
		}
		lastID = id
	}
	if lastID == 0 {
		t.Fatal("no message id")
	}
	// sendMessage can return the id before getHistory sees the row. The
	// clear anchor is the newest stored message, so message 3 has to be
	// visible to both sessions of this account before deleteHistory.
	waitHistoryID(t, a, peer, lastID)
	waitHistoryID(t, a2, peer, lastID)

	pts, qts, date, err := a2.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        peer,
		JustClear:   true,
		Revoke:      true,
		MaxId:       0,
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(30 * time.Second)
	var diff mtproto.TLObject
	for {
		diff, err = a2.Difference(ctx, pts, qts, date)
		if err != nil {
			t.Fatal(err)
		}
		hist, err := a2.History(ctx, peer, 20)
		if err != nil {
			t.Fatal(err)
		}
		if differenceDeletes(diff, lastID) && historyTopIsClear(hist) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("other session missed delete of %d: %s history %d", lastID, summarizeUpdates(diff), len(hist))
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(400 * time.Millisecond):
		}
	}
}

func waitHistoryID(t *testing.T, c *Client, peer *mtproto.InputPeer, id int32) {
	t.Helper()
	ctx, cancel := contextTimeout(10 * time.Second)
	defer cancel()
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		hist, err := c.History(ctx, peer, 20)
		if err != nil {
			if ctx.Err() != nil {
				return false, nil
			}
			return false, err
		}
		for _, m := range hist {
			if m != nil && m.GetId() == id {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("history never showed message %d: %v", id, err)
	}
}

func historyTopIsClear(msgs []*mtproto.Message) bool {
	for _, m := range msgs {
		if m == nil || m.GetPredicateName() == mtproto.Predicate_messageEmpty {
			continue
		}
		return isHistoryClear(m)
	}
	return false
}
