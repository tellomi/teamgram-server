package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestDraftAndUnreadMarkAreVisibleOnTheOtherSession covers
// ISSUE-S-DRAFT-DIALOG-001 and ISSUE-S-UNREADMARK-XSYNC-001.
// Phones 9996610478 and 9996610479.
func TestDraftAndUnreadMarkAreVisibleOnTheOtherSession(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610478", "SyncA")
	b, _ := dialPhone(t, ctx, cfg, "9996610478", "SyncA")
	peerUser, _ := dialPhone(t, ctx, cfg, "9996610479", "SyncB")
	_ = peerUser
	peerID, peerHash, err := a.ImportPhone(ctx, "9996610479", "SyncB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(peerID, peerHash)
	if _, err = a.SendText(ctx, peer, fmt.Sprintf("seed-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Dialogs(ctx); err != nil {
		t.Fatal(err)
	}
	text := "draft-from-a"
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesSaveDraft{
		Constructor: mtproto.CRC32_messages_saveDraft_bc39e14b,
		Peer:        peer,
		Message:     text,
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		dialogs, err := b.Dialogs(ctx)
		if err != nil {
			return false, err
		}
		d := dialogByUser(dialogs, peerID)
		return d != nil && d.GetDraft().GetMessage() == text, nil
	}); err != nil {
		t.Fatalf("other session draft: %v", err)
	}

	if _, err = a.Invoke(ctx, &mtproto.TLMessagesMarkDialogUnread{
		Constructor: mtproto.CRC32_messages_markDialogUnread_c286d98f,
		Unread:      true,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: peer,
		}).To_InputDialogPeer(),
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		dialogs, err := b.Dialogs(ctx)
		if err != nil {
			return false, err
		}
		d := dialogByUser(dialogs, peerID)
		return d != nil && d.GetUnreadMark(), nil
	}); err != nil {
		t.Fatalf("other session unread mark: %v", err)
	}
	_ = aSelf
}

func dialogByUser(dialogs []*mtproto.Dialog, userID int64) *mtproto.Dialog {
	for _, d := range dialogs {
		if d.GetPeer().GetUserId() == userID {
			return d
		}
	}
	return nil
}
