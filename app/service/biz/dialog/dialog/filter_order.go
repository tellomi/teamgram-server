package dialog

// DialogFilterDefaultID is dialogFilterDefault, the "All chats" folder.
// messages.updateDialogFiltersOrder puts 0 in the order vector for it.
// messages.updateDialogFilter rejects this id.
const DialogFilterDefaultID int32 = 0

// FilterOrderStep is one id from messages.updateDialogFiltersOrder.
// OrderValue decreases from the front of the vector because stored lists are
// read with ORDER BY order_value DESC, so the first id sorts first.
// UpsertDefault is set for id 0. That row does not exist until the user moves
// All chats, and UPDATE alone would not record the position.
type FilterOrderStep struct {
	ID            int32
	OrderValue    int64
	UpsertDefault bool
}

// FilterOrderSteps assigns a descending order_value to each id.
// nowUnix is seconds; the high bits keep a later reorder ahead of rows that
// were inserted with time.Now().Unix()<<32.
func FilterOrderSteps(order []int32, nowUnix int64) []FilterOrderStep {
	if len(order) == 0 {
		return nil
	}
	orderV := nowUnix << 32
	out := make([]FilterOrderStep, len(order))
	for i, id := range order {
		out[i] = FilterOrderStep{
			ID:            id,
			OrderValue:    orderV,
			UpsertDefault: id == DialogFilterDefaultID,
		}
		orderV--
	}
	return out
}
