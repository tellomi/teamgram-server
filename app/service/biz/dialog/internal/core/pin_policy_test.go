package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

func TestReorderWithoutForceKeepsUnlistedPins(t *testing.T) {
	current := []int64{10, 20}
	order := []int64{10}
	if reorderPinCount(false, current, order) != 2 {
		t.Fatalf("kept %d", reorderPinCount(false, current, order))
	}
	if err := reorderPinsRejected(false, current, order, 5); err != nil {
		t.Fatal(err)
	}
	if reorderPinCount(true, current, order) != 1 {
		t.Fatal("force should drop the pin the client left out")
	}
}

func TestReorderRejectsAnOrderPastTheLimit(t *testing.T) {
	order := []int64{1, 2, 3, 4, 5, 6}
	if err := reorderPinsRejected(true, nil, order, premium.PinnedDialogsFree); err != mtproto.ErrPinnedDialogsTooMuch {
		t.Fatalf("err %v", err)
	}
	if err := reorderPinsRejected(true, nil, order, premium.PinnedDialogs); err != nil {
		t.Fatal(err)
	}
}

func TestPinLimitFollowsMembership(t *testing.T) {
	t.Cleanup(func() { premium.Reload() })
	t.Setenv(premium.EnvEveryone, "false")
	premium.Reload()

	free := premium.Limit(premium.FeaturePinnedChats, false)
	member := premium.Limit(premium.FeaturePinnedChats, true)
	if free != premium.PinnedDialogsFree || member != premium.PinnedDialogs {
		t.Fatalf("free %d member %d", free, member)
	}
	if err := pinRejected(false, free, free); err != mtproto.ErrPinnedDialogsTooMuch {
		t.Fatalf("sixth free pin: %v", err)
	}
	if err := pinRejected(false, free, member); err != nil {
		t.Fatalf("sixth member pin: %v", err)
	}
	if pinnedCap(nil, 1) != free {
		t.Fatalf("nil core without Everyone uses the free cap %d", pinnedCap(nil, 1))
	}

	t.Setenv(premium.EnvEveryone, "true")
	premium.Reload()
	if pinnedCap(nil, 1) != premium.PinnedDialogs {
		t.Fatalf("Everyone cap %d", pinnedCap(nil, 1))
	}
	if pinnedCapFor(nil, 1, 1) != premium.FolderPinnedDialogs {
		t.Fatalf("Everyone folder cap %d", pinnedCapFor(nil, 1, 1))
	}
	if err := pinRejected(false, 5, pinnedCap(nil, 1)); err != nil {
		t.Fatalf("Everyone sixth pin: %v", err)
	}
}
