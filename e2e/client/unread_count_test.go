package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestUnreadCountIsPerDialogMessageCount is the server side of the Android
// screenshot where one chat shows 4 and the Chats tab shows 1.
//
// Official Android (DrKLO/Telegram, MessagesStorage.calcUnreadCounters) adds
// one to the in-app Chats tab for each dialog with unread_count > 0. The row
// paints dialog.unread_count. updates.getState.unread_count is not read by
// that client (loadCurrentState stores pts, qts, seq and date only). A tab
// of 1 next to a row of 4 is that client setting, as long as the dialog
// itself reports 4.
func TestUnreadCountIsPerDialogMessageCount(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)

	if d, ok, err := dialogOf(b, ctx, "user", aliceSeen.ID); err != nil {
		t.Fatal(err)
	} else if ok && d.GetTopMessage() > 0 {
		if err := b.ReadHistory(ctx, peerForB, d.GetTopMessage()); err != nil {
			t.Fatal(err)
		}
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		d, ok, err := dialogOf(b, ctx, "user", aliceSeen.ID)
		if err != nil || !ok {
			return false, err
		}
		return d.GetUnreadCount() == 0, nil
	}); err != nil {
		t.Fatal(err)
	}

	pts, qts, date, err := b.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int32
	for i := 0; i < 4; i++ {
		text := fmt.Sprintf("unread-%d-%d", i, time.Now().UnixNano())
		if _, err := a.SendText(ctx, peerForA, text, nil); err != nil {
			t.Fatal(err)
		}
		if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
			msgs, err := b.History(ctx, peerForB, 20)
			if err != nil {
				return false, err
			}
			id := idOfText(msgs, text)
			if id == 0 {
				return false, nil
			}
			ids = append(ids, id)
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		d, ok, err := dialogOf(b, ctx, "user", aliceSeen.ID)
		if err != nil || !ok {
			return false, err
		}
		return d.GetUnreadCount() == 4 && d.GetReadInboxMaxId() < ids[0], nil
	}); err != nil {
		d, _, _ := dialogOf(b, ctx, "user", aliceSeen.ID)
		t.Fatalf("dialog unread_count=%d read_inbox=%d, want 4 unread messages: %v", d.GetUnreadCount(), d.GetReadInboxMaxId(), err)
	}

	if err := b.ReadHistory(ctx, peerForB, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		d, ok, err := dialogOf(b, ctx, "user", aliceSeen.ID)
		if err != nil || !ok {
			return false, err
		}
		return d.GetUnreadCount() == 3 && d.GetReadInboxMaxId() >= ids[0], nil
	}); err != nil {
		d, _, _ := dialogOf(b, ctx, "user", aliceSeen.ID)
		t.Fatalf("after reading one, unread=%d read_inbox=%d, want 3 and inbox>=%d: %v", d.GetUnreadCount(), d.GetReadInboxMaxId(), ids[0], err)
	}
	diff, err := b.Difference(ctx, pts, qts, date)
	if err != nil {
		t.Fatal(err)
	}
	if !differenceStillUnread(diff, aliceSeen.ID, 3) {
		t.Fatalf("getDifference missing updateReadHistoryInbox still_unread_count=3")
	}
}

func differenceStillUnread(obj mtproto.TLObject, peerUser int64, want int32) bool {
	for _, up := range differenceUpdates(obj) {
		if up.GetPredicateName() != mtproto.Predicate_updateReadHistoryInbox {
			continue
		}
		if up.GetPeer_PEER().GetUserId() != peerUser {
			continue
		}
		if up.GetStillUnreadCount() == want && up.GetPts_INT32() > 0 {
			return true
		}
	}
	return false
}
