package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestDialogSurvivesDeletingTheLatestMessage is ISSUE-S-CHAN-TOPDEL-001 and
// the same check for a private chat and a basic group. The chat stays, and
// its top is the previous message. Deleting every message leaves a private
// chat off the list and leaves a group or supergroup the user still belongs
// to on the list with top_message 0.
//
// ISSUE-S-PIN-CHAN-DIALOGS-001: pinning the supergroup sets pinned on
// messages.getDialogs, puts it ahead of a newer unpinned chat, returns it
// from messages.getPinnedDialogs, and pushes updateDialogPinned with the
// channel to the account's other session.
func TestDialogSurvivesDeletingTheLatestMessage(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "dlgpin_a", "PinA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "dlgpin_b", "PinB")
	other, _ := loginNamed(t, cfg, ProfileAndroid, "dlgpin_a", "PinA")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	chID, chHash, err := a.CreateMegagroup(ctx, fmt.Sprintf("pin-top-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	chPeer := inputPeerChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	older, err := a.SendText(ctx, chPeer, "older", nil)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := a.SendText(ctx, chPeer, "newer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		top, ok := dialogByPeer(t, ctx, b, "channel", chID)
		return ok && top.GetTopMessage() == newer, nil
	}); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLChannelsDeleteMessages{
		Constructor: mtproto.CRC32_channels_deleteMessages,
		Channel:     channel,
		Id:          []int32{newer},
	})
	for _, who := range []*Client{a, b} {
		row, ok := dialogByPeer(t, ctx, who, "channel", chID)
		if !ok || row.GetTopMessage() != older {
			t.Fatalf("top after deleting the latest = %d present %v, want %d", row.GetTopMessage(), ok, older)
		}
	}

	// Creating the channel and deleting its latest message takes longer than
	// the online record. Sync only pushes to sessions still in that list, so
	// the other login has to touch the server immediately before the pin.
	if _, _, _, err = other.State(ctx); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLMessagesToggleDialogPin{
		Constructor: mtproto.CRC32_messages_toggleDialogPin,
		Pinned:      true,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: chPeer,
		}).To_InputDialogPeer(),
	})
	pinned, ok := dialogByPeer(t, ctx, a, "channel", chID)
	if !ok || !pinned.GetPinned() {
		t.Fatalf("getDialogs pinned=%v present=%v", pinned.GetPinned(), ok)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return pushedDialogPin(other.Pushed(), chID), nil
	}); err != nil {
		t.Fatal(err)
	}
	fresh, _ := loginNamed(t, cfg, ProfileIOS, "dlgpin_a", "PinA")
	again, ok := dialogByPeer(t, ctx, fresh, "channel", chID)
	if !ok || !again.GetPinned() || again.GetTopMessage() != older {
		t.Fatalf("fresh session dialog pinned=%v top=%d", again.GetPinned(), again.GetTopMessage())
	}
	pins := invokeOK(t, a, &mtproto.TLMessagesGetPinnedDialogs{
		Constructor: mtproto.CRC32_messages_getPinnedDialogs,
		FolderId:    0,
	})
	foundPin := false
	for _, d := range dialogsOf(pins) {
		if d != nil && d.GetPeer().GetChannelId() == chID && d.GetPinned() && d.GetTopMessage() == older {
			foundPin = true
		}
	}
	if !foundPin {
		t.Fatalf("getPinnedDialogs missed the channel: %T", pins)
	}
	userPeer := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	if _, err = a.SendText(ctx, userPeer, fmt.Sprintf("newer-than-pin-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	page, err := a.dialogPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := firstListedDialog(page.Dialogs)
	if first == nil || first.GetPeer().GetChannelId() != chID {
		t.Fatalf("first dialog %#v, want pinned channel %d", first.GetPeer(), chID)
	}
	hidden := invokeOK(t, a, &mtproto.TLMessagesGetDialogs{
		Constructor:   mtproto.CRC32_messages_getDialogs,
		ExcludePinned: true,
		OffsetPeer:    inputPeerEmpty(),
		Limit:         100,
	})
	if _, ok := findDialogInList(dialogsOf(hidden), "channel", chID); ok {
		t.Fatal("exclude_pinned still returned the pinned channel")
	}

	hist, err := a.History(ctx, chPeer, 50)
	if err != nil {
		t.Fatal(err)
	}
	var all []int32
	for _, m := range hist {
		if m != nil && m.GetId() != 0 {
			all = append(all, m.GetId())
		}
	}
	if len(all) == 0 {
		t.Fatal("channel history was already empty")
	}
	invokeOK(t, a, &mtproto.TLChannelsDeleteMessages{
		Constructor: mtproto.CRC32_channels_deleteMessages,
		Channel:     channel,
		Id:          all,
	})
	empty, ok := dialogByPeer(t, ctx, a, "channel", chID)
	if !ok || empty.GetTopMessage() != 0 || !empty.GetPinned() {
		t.Fatalf("empty supergroup dialog present=%v top=%d pinned=%v", ok, empty.GetTopMessage(), empty.GetPinned())
	}
	member, ok := dialogByPeer(t, ctx, b, "channel", chID)
	if !ok || member.GetTopMessage() != 0 {
		t.Fatalf("member empty dialog present=%v top=%d", ok, member.GetTopMessage())
	}

	p1, err := a.SendText(ctx, userPeer, "pm-older", nil)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := a.SendText(ctx, userPeer, "pm-newer", nil)
	if err != nil {
		t.Fatal(err)
	}
	// sendMessage returns the id before the inbox consumer inserts the row.
	// Deleting in that gap is a no-op and the dialog top stays on the new id.
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		hist, err := a.History(ctx, userPeer, 20)
		if err != nil {
			return false, err
		}
		for _, m := range hist {
			if m.GetId() == p2 {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{p2},
	})
	pm, ok := dialogByPeer(t, ctx, a, "user", bSelf.ID)
	if !ok || pm.GetTopMessage() != p1 {
		t.Fatalf("private top present=%v top=%d, want %d", ok, pm.GetTopMessage(), p1)
	}
	pmHist, err := a.History(ctx, userPeer, 50)
	if err != nil {
		t.Fatal(err)
	}
	var pmIDs []int32
	for _, m := range pmHist {
		if m != nil && m.GetId() != 0 {
			pmIDs = append(pmIDs, m.GetId())
		}
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          pmIDs,
	})
	if _, ok = dialogByPeer(t, ctx, a, "user", bSelf.ID); ok {
		t.Fatal("private chat with no messages stayed in getDialogs")
	}

	chatID, err := a.CreateChat(ctx, fmt.Sprintf("pin-group-%d", time.Now().UnixNano()), inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	gPeer := inputPeerChat(chatID)
	g1, err := a.SendText(ctx, gPeer, "g-older", nil)
	if err != nil {
		t.Fatal(err)
	}
	g2, err := a.SendText(ctx, gPeer, "g-newer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		hist, err := a.History(ctx, gPeer, 20)
		if err != nil {
			return false, err
		}
		for _, m := range hist {
			if m.GetId() == g2 {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{g2},
	})
	group, ok := dialogByPeer(t, ctx, a, "chat", chatID)
	if !ok || group.GetTopMessage() != g1 {
		t.Fatalf("group top present=%v top=%d, want %d", ok, group.GetTopMessage(), g1)
	}
	gh, err := a.History(ctx, gPeer, 50)
	if err != nil {
		t.Fatal(err)
	}
	var gIDs []int32
	for _, m := range gh {
		if m != nil && m.GetId() != 0 {
			gIDs = append(gIDs, m.GetId())
		}
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          gIDs,
	})
	left, ok := dialogByPeer(t, ctx, a, "chat", chatID)
	if !ok || left.GetTopMessage() != 0 {
		t.Fatalf("empty group present=%v top=%d", ok, left.GetTopMessage())
	}
}

func dialogByPeer(t *testing.T, ctx context.Context, c *Client, kind string, id int64) (*mtproto.Dialog, bool) {
	t.Helper()
	page, err := c.dialogPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return findDialogInList(page.Dialogs, kind, id)
}

func findDialogInList(dialogs []*mtproto.Dialog, kind string, id int64) (*mtproto.Dialog, bool) {
	want := kind + "/" + fmt.Sprint(id)
	for _, d := range dialogs {
		if d != nil && d.GetPeer() != nil && peerKindID(d.GetPeer()) == want {
			return d, true
		}
	}
	return nil, false
}

func firstListedDialog(dialogs []*mtproto.Dialog) *mtproto.Dialog {
	for _, d := range dialogs {
		if d == nil || d.GetPredicateName() == mtproto.Predicate_dialogFolder {
			continue
		}
		return d
	}
	return nil
}

func pushedDialogPin(objs []mtproto.TLObject, channelID int64) bool {
	for _, obj := range objs {
		var ups []*mtproto.Update
		var chats []*mtproto.Chat
		switch v := obj.(type) {
		case *mtproto.TLUpdates:
			ups = v.GetUpdates()
			chats = v.GetChats()
		case *mtproto.TLUpdatesCombined:
			ups = v.GetUpdates()
			chats = v.GetChats()
		case *mtproto.TLUpdateShort:
			if v.GetUpdate() != nil {
				ups = []*mtproto.Update{v.GetUpdate()}
			}
		}
		for _, u := range ups {
			if u.GetPredicateName() != mtproto.Predicate_updateDialogPinned || !u.GetPinned() {
				continue
			}
			peer := u.GetPeer_DIALOGPEER().GetPeer()
			if peer.GetChannelId() != channelID {
				continue
			}
			for _, ch := range chats {
				if ch != nil && ch.GetId() == channelID {
					return true
				}
			}
		}
	}
	return false
}
