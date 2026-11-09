package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestOwnStatusIsNotPushedToOtherSessions is the official account.updateStatus
// rule. Contacts learn the new status. The same user's other devices do not
// get updateUserStatus for themselves. A second identical call must not fail.
func TestOwnStatusIsNotPushedToOtherSessions(t *testing.T) {
	a, b, aSelf, _ := sharedPair(t)
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(45 * time.Second)
	defer cancel()
	phone, err := cfg.Phone("alice")
	if err != nil {
		t.Fatal(err)
	}
	other, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.Close)
	if _, err = other.Login(ctx, phone, "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = other.State(ctx); err != nil {
		t.Fatal(err)
	}
	beforeOther := len(other.Pushed())
	beforeBob := len(b.Pushed())
	call := func() error {
		_, err := a.Invoke(ctx, &mtproto.TLAccountUpdateStatus{
			Constructor: mtproto.CRC32_account_updateStatus,
			Offline:     mtproto.MakeTLBoolFalse(nil).To_Bool(),
		})
		return err
	}
	if err = call(); err != nil {
		t.Fatal(err)
	}
	if err = call(); err != nil {
		t.Fatalf("repeat updateStatus: %v", err)
	}
	err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return sawUserStatusFor(pushedSince(b.Pushed(), beforeBob), aSelf.ID), nil
	})
	if err != nil {
		t.Fatalf("contact did not receive updateUserStatus: %v", err)
	}
	if sawUserStatusFor(pushedSince(other.Pushed(), beforeOther), aSelf.ID) {
		t.Fatal("another session of the same user received updateUserStatus")
	}
}

func pushedSince(objs []mtproto.TLObject, n int) []mtproto.TLObject {
	if n < 0 || n > len(objs) {
		return objs
	}
	return objs[n:]
}

func sawUserStatusFor(objs []mtproto.TLObject, userID int64) bool {
	for _, obj := range objs {
		var ups []*mtproto.Update
		switch v := obj.(type) {
		case *mtproto.TLUpdates:
			ups = v.GetUpdates()
		case *mtproto.TLUpdatesCombined:
			ups = v.GetUpdates()
		case *mtproto.TLUpdateShort:
			ups = []*mtproto.Update{v.GetUpdate()}
		case *mtproto.Updates:
			ups = v.GetUpdates()
			if v.GetUpdate() != nil {
				ups = append(ups, v.GetUpdate())
			}
		}
		for _, u := range ups {
			if u.GetPredicateName() == mtproto.Predicate_updateUserStatus && (userID == 0 || u.GetUserId() == userID) {
				return true
			}
		}
	}
	return false
}

// TestChannelReadInboxSurvivesReconnect checks the reader, not only the sender.
// After channels.readHistory, a fresh login of the reader still has the inbox
// cursor, and reading the same id again succeeds.
func TestChannelReadInboxSurvivesReconnect(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(60 * time.Second)
	defer cancel()
	chID, chHash, err := a.CreateMegagroup(ctx, fmt.Sprintf("e2e-inbox-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("inbox-%d", time.Now().UnixNano())
	if _, err = a.SendText(ctx, inputPeerChannel(chID, chHash), text, nil); err != nil {
		t.Fatal(err)
	}
	var id int32
	var readerHash int64
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, inputPeerChannel(chID, chHash), 20)
		if err != nil {
			return false, err
		}
		id = idOfText(msgs, text)
		got, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		readerHash = got
		return id != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = b.ReadChannelHistory(ctx, inputChannel(chID, readerHash), id); err != nil {
		t.Fatal(err)
	}
	if err = b.ReadChannelHistory(ctx, inputChannel(chID, readerHash), id); err != nil {
		t.Fatalf("repeat read: %v", err)
	}
	phone, err := cfg.Phone("bob")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	if _, err = again.Login(ctx, phone, "Bob"); err != nil {
		t.Fatal(err)
	}
	inbox, ok, err := again.InboxMaxID(ctx, "channel", chID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || inbox < id {
		t.Fatalf("reconnected inbox %d ok %v want >= %d", inbox, ok, id)
	}
}
