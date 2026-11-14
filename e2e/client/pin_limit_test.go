package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestPinnedDialogsStopAtTheAppConfigCap is ISSUE-S-PIN-LIMIT-001.
// messages.toggleDialogPin and messages.reorderPinnedDialogs reject a pin
// set past dialogs_pinned_limit_default or _premium, and the archive uses
// dialogs_folder_pinned_limit_default or _premium. The error is
// PINNED_DIALOGS_TOO_MUCH. A pin under the cap still sticks, and a rejected
// reorder does not clear it.
func TestPinnedDialogsStopAtTheAppConfigCap(t *testing.T) {
	cfg := requireE2E(t)
	a, self := loginNamed(t, cfg, ProfileIOS, "pinlim_a", "PinLim")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	app := appConfigMap(t, invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{
		Constructor: mtproto.CRC32_help_getAppConfig61E3F854,
	}))
	member := selfPremium(t, a, self.ID)
	limit := advertisedPinLimit(t, app, "dialogs_pinned_limit", member)
	folderLimit := advertisedPinLimit(t, app, "dialogs_folder_pinned_limit", member)
	if limit < 1 || folderLimit < 1 {
		t.Fatalf("advertised caps main %d folder %d", limit, folderLimit)
	}

	clearPins(t, a, 0)
	clearPins(t, a, 1)
	chatID, err := a.CreateChat(ctx, fmt.Sprintf("pin-cap-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	togglePin(t, a, chatID, true)
	row, ok := dialogByPeer(t, ctx, a, "chat", chatID)
	if !ok || !row.GetPinned() {
		t.Fatalf("pin under the cap present=%v pinned=%v", ok, row.GetPinned())
	}

	_, err = a.Invoke(ctx, reorderPins(true, 0, fakeDialogChats(limit+1)))
	wantRPC(t, err, 400, "PINNED_DIALOGS_TOO_MUCH")
	_, err = a.Invoke(ctx, reorderPins(false, 0, fakeDialogChats(limit)))
	wantRPC(t, err, 400, "PINNED_DIALOGS_TOO_MUCH")
	_, err = a.Invoke(ctx, reorderPins(true, 1, fakeDialogChats(folderLimit+1)))
	wantRPC(t, err, 400, "PINNED_DIALOGS_TOO_MUCH")
	row, ok = dialogByPeer(t, ctx, a, "chat", chatID)
	if !ok || !row.GetPinned() {
		t.Fatal("a rejected reorder cleared the pin")
	}

	if limit <= 6 {
		fillPinsUntil(t, ctx, a, limit, chatID)
		extra, err := a.CreateChat(ctx, fmt.Sprintf("pin-extra-%d", time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.Invoke(ctx, togglePinReq(extra, true))
		wantRPC(t, err, 400, "PINNED_DIALOGS_TOO_MUCH")
		togglePin(t, a, chatID, false)
		togglePin(t, a, extra, true)
		row, ok = dialogByPeer(t, ctx, a, "chat", extra)
		if !ok || !row.GetPinned() {
			t.Fatal("a pin opened by unpinning another chat did not stick")
		}
	}
}

func advertisedPinLimit(t *testing.T, app map[string]any, key string, member bool) int {
	t.Helper()
	suffix := "_default"
	if member {
		suffix = "_premium"
	}
	v, ok := app[key+suffix].(float64)
	if !ok || v < 0 || v != float64(int(v)) {
		t.Fatalf("%s%s = %#v", key, suffix, app[key+suffix])
	}
	return int(v)
}

func selfPremium(t *testing.T, c *Client, self int64) bool {
	t.Helper()
	full := invokeOK(t, c, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUserSelf(),
	})
	for _, u := range usersOf(full) {
		if u.GetId() == self {
			return u.GetPremium()
		}
	}
	t.Fatal("getFullUser did not return the account")
	return false
}

func clearPins(t *testing.T, c *Client, folder int32) {
	t.Helper()
	invokeOK(t, c, reorderPins(true, folder, nil))
}

func togglePin(t *testing.T, c *Client, chatID int64, pinned bool) {
	t.Helper()
	invokeOK(t, c, togglePinReq(chatID, pinned))
}

func togglePinReq(chatID int64, pinned bool) *mtproto.TLMessagesToggleDialogPin {
	return &mtproto.TLMessagesToggleDialogPin{
		Constructor: mtproto.CRC32_messages_toggleDialogPin,
		Pinned:      pinned,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: inputPeerChat(chatID),
		}).To_InputDialogPeer(),
	}
}

func reorderPins(force bool, folder int32, order []*mtproto.InputDialogPeer) *mtproto.TLMessagesReorderPinnedDialogs {
	return &mtproto.TLMessagesReorderPinnedDialogs{
		Constructor: mtproto.CRC32_messages_reorderPinnedDialogs,
		Force:       force,
		FolderId:    folder,
		Order:       order,
	}
}

func fakeDialogChats(n int) []*mtproto.InputDialogPeer {
	order := make([]*mtproto.InputDialogPeer, n)
	for i := range order {
		order[i] = mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: inputPeerChat(int64(900000 + i)),
		}).To_InputDialogPeer()
	}
	return order
}

func fillPinsUntil(t *testing.T, ctx context.Context, c *Client, limit int, have int64) {
	t.Helper()
	n := 1
	for n < limit {
		id, err := c.CreateChat(ctx, fmt.Sprintf("pin-fill-%d-%d", n, time.Now().UnixNano()))
		if err != nil {
			t.Fatal(err)
		}
		togglePin(t, c, id, true)
		n++
	}
	row, ok := dialogByPeer(t, ctx, c, "chat", have)
	if !ok || !row.GetPinned() {
		t.Fatal("the first pin disappeared while filling the cap")
	}
}
