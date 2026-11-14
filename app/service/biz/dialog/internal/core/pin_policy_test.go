package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPinnedDialogLimitMatchesConfig(t *testing.T) {
	if pinnedDialogLimit(0) != 5 || pinnedDialogLimit(1) != 100 {
		t.Fatalf("main %d folder %d", pinnedDialogLimit(0), pinnedDialogLimit(1))
	}
}

func TestReorderWithoutForceRequiresEveryCurrentPin(t *testing.T) {
	err := reorderPinsRejected(false, []int64{10, 20}, []int64{10}, 5)
	if err != mtproto.ErrPinnedDialogsTooMuch {
		t.Fatalf("err %v", err)
	}
	if err := reorderPinsRejected(false, []int64{10, 20}, []int64{20, 10}, 5); err != nil {
		t.Fatal(err)
	}
	if err := reorderPinsRejected(true, []int64{10, 20}, []int64{10}, 5); err != nil {
		t.Fatal(err)
	}
}

func TestReorderRejectsAnOrderPastTheLimit(t *testing.T) {
	order := []int64{1, 2, 3, 4, 5, 6}
	if err := reorderPinsRejected(true, nil, order, 5); err != mtproto.ErrPinnedDialogsTooMuch {
		t.Fatalf("err %v", err)
	}
}
