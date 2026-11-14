package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMarkDialogUnreadClearsAndReachesTheOtherSession covers
// ISSUE-S-UNREADMARK-001 (row 78). Marking a dialog unread stuck because
// getDialogs kept a cached DialogExt, and the other session never got
// updateDialogUnreadMark.
func TestMarkDialogUnreadClearsAndReachesTheOtherSession(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610443", "MarkA")
	other, _ := dialPhone(t, ctx, cfg, "9996610443", "MarkA2")
	_, _ = dialPhone(t, ctx, cfg, "9996610444", "MarkB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610444", "MarkB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bID, bHash)
	if _, err = a.SendText(ctx, peer, "mark-me", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = other.State(ctx); err != nil {
		t.Fatal(err)
	}

	markDialogUnread(t, a, peer, true)
	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return dialogUnread(t, other, bID) && pushedUnreadMark(other.Pushed(), bID, true), nil
	}); err != nil {
		t.Fatalf("other session did not see the unread mark: %v; pushed=%s", err, pushedSummary(other.Pushed()))
	}

	markDialogUnread(t, a, peer, false)
	if err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		cleared := !dialogUnread(t, a, bID) && !dialogUnread(t, other, bID)
		return cleared && pushedUnreadMark(other.Pushed(), bID, false), nil
	}); err != nil {
		t.Fatalf("unread mark did not clear: %v; pushed=%s", err, pushedSummary(other.Pushed()))
	}
}

func markDialogUnread(t *testing.T, c *Client, peer *mtproto.InputPeer, unread bool) {
	t.Helper()
	invokeOK(t, c, &mtproto.TLMessagesMarkDialogUnread{
		Constructor: mtproto.CRC32_messages_markDialogUnread_c286d98f,
		Unread:      unread,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: peer,
		}).To_InputDialogPeer(),
	})
}

func dialogUnread(t *testing.T, c *Client, userID int64) bool {
	t.Helper()
	ctx, cancel := contextTimeout(15 * time.Second)
	defer cancel()
	page, err := c.dialogPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range page.Dialogs {
		if d.GetPeer().GetUserId() == userID {
			return d.GetUnreadMark()
		}
	}
	return false
}

func pushedUnreadMark(objs []mtproto.TLObject, userID int64, unread bool) bool {
	for _, obj := range objs {
		for _, upd := range updatesOf(obj) {
			if upd.GetPredicateName() != mtproto.Predicate_updateDialogUnreadMark || upd.GetUnread() != unread {
				continue
			}
			if upd.GetPeer_DIALOGPEER().GetPeer().GetUserId() == userID {
				return true
			}
		}
	}
	return false
}
