package core

import "github.com/teamgram/proto/mtproto"

// pinRejected is messages.toggleDialogPin past the account cap.
// alreadyPinned is a no-op and does not consume a slot.
func pinRejected(alreadyPinned bool, currentCount, limit int) error {
	if alreadyPinned || limit < 0 || currentCount < limit {
		return nil
	}
	return mtproto.ErrPinnedDialogsTooMuch
}

// reorderPinCount is how many dialogs stay pinned after messages.reorderPinnedDialogs.
// force drops pins the client left out. Without force those pins stay, after
// the order the client sent. Only a result past the cap is PINNED_DIALOGS_TOO_MUCH.
func reorderPinCount(force bool, current, order []int64) int {
	if force {
		return len(order)
	}
	have := make(map[int64]struct{}, len(order))
	for _, id := range order {
		have[id] = struct{}{}
	}
	n := len(order)
	for _, id := range current {
		if _, ok := have[id]; !ok {
			n++
		}
	}
	return n
}

func reorderPinsRejected(force bool, current, order []int64, limit int) error {
	if limit >= 0 && reorderPinCount(force, current, order) > limit {
		return mtproto.ErrPinnedDialogsTooMuch
	}
	return nil
}
