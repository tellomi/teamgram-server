package dao

// dialogTopCandidate is one live message considered for the dialog top.
// Newest comes first.
type dialogTopCandidate struct {
	ID   int32
	Date int64
}

// remainingDialogTop is the newest message that this delete does not remove.
// newest is the live rows loaded for the dialog, newest id first, and it
// includes one message past the delete set when any older message exists.
// The zero id means nothing live remains, which is how a private chat leaves
// the chat list.
func remainingDialogTop(newest []dialogTopCandidate, deleted []int32) (int32, int64) {
	drop := make(map[int32]struct{}, len(deleted))
	for _, id := range deleted {
		drop[id] = struct{}{}
	}
	for _, m := range newest {
		if m.ID <= 0 {
			continue
		}
		if _, ok := drop[m.ID]; ok {
			continue
		}
		return m.ID, m.Date
	}
	return 0, 0
}

// deletedTouchesTop reports whether the dialog top has to be recomputed.
// A delete of an older message leaves the top where it is.
func deletedTouchesTop(top int32, deleted []int32) bool {
	for _, id := range deleted {
		if id >= top && id > 0 {
			return true
		}
	}
	return false
}
