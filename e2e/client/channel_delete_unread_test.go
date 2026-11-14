package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMegagroupDeleteUnreadDropsDialogCount deletes another member's
// messages that the caller has not read. The dialog unread_count falls by
// the number of those messages, the same way a private revoke does.
func TestMegagroupDeleteUnreadDropsDialogCount(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	title := fmt.Sprintf("unread-del-%d", time.Now().UnixNano())
	chID, chHash, err := a.CreateMegagroup(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	var bHash int64
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		got, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		bHash = got
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	peerForB := inputPeerChannel(chID, bHash)
	text1 := fmt.Sprintf("del-unread-a-%d", time.Now().UnixNano())
	text2 := fmt.Sprintf("del-unread-b-%d", time.Now().UnixNano())
	if _, err = b.SendText(ctx, peerForB, text1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = b.SendText(ctx, peerForB, text2, nil); err != nil {
		t.Fatal(err)
	}
	peerForA := inputPeerChannel(chID, chHash)
	var first int32
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		d, ok, err := dialogOf(a, ctx, "channel", chID)
		if err != nil || !ok {
			return false, err
		}
		if d.GetUnreadCount() != 2 {
			return false, nil
		}
		msgs, err := a.History(ctx, peerForA, 20)
		if err != nil {
			return false, err
		}
		first = idOfText(msgs, text1)
		return first != 0, nil
	}); err != nil {
		d, _, _ := dialogOf(a, ctx, "channel", chID)
		t.Fatalf("dialog unread %d, want 2: %v", d.GetUnreadCount(), err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLChannelsDeleteMessages{
		Constructor: mtproto.CRC32_channels_deleteMessages,
		Channel:     channel,
		Id:          []int32{first},
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		d, ok, err := dialogOf(a, ctx, "channel", chID)
		if err != nil || !ok {
			return false, err
		}
		return d.GetUnreadCount() == 1, nil
	}); err != nil {
		d, _, _ := dialogOf(a, ctx, "channel", chID)
		t.Fatalf("dialog unread %d after delete, want 1: %v", d.GetUnreadCount(), err)
	}
}
