package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestOtherSessionReceivesOnlineStatus is the regression for account.updateStatus.
// One login goes online. The other login of the same account must see
// updateUserStatus, and a second identical call must not fail.
func TestOtherSessionReceivesOnlineStatus(t *testing.T) {
	a, _, _, _ := sharedPair(t)
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
	_ = other.Pushed()
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
		return sawUserStatus(other.Pushed()), nil
	})
	if err != nil {
		t.Fatalf("other session did not receive updateUserStatus: %v", err)
	}
}

func sawUserStatus(objs []mtproto.TLObject) bool {
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
			if u.GetPredicateName() == mtproto.Predicate_updateUserStatus {
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
