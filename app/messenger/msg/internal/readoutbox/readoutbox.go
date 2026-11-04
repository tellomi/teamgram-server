// Package readoutbox decides whose outbox advances when someone reads.
// A user's own read never marks their messages read.
package readoutbox

// Message is one message inside the range a reader has just seen.
type Message struct {
	SenderID        int64
	DialogMessageID int64
}

// Target is one sender whose outbox should move to DialogMessageID.
type Target struct {
	SenderID        int64
	DialogMessageID int64
}

// Advance reports that next is a strictly newer outbox cursor.
// Equal and older ids must not move read_outbox_max_id backwards.
func Advance(current, next int32) bool {
	return next > 0 && next > current
}

// NotifySender reports that reader has seen a message written by sender.
// The reader's inbox box id and the reader's own ReadOutboxMaxId are different
// cursors in the same box. An incoming message often has a lower box id than
// a later outgoing message the peer already read, so comparing the two drops
// a real read receipt. Callers must not make that comparison.
func NotifySender(reader, sender int64) bool {
	return reader != 0 && sender != 0 && sender != reader
}

// Targets returns the other participants whose messages reader has seen.
// The highest dialog message id wins for each sender. The reader's own
// messages are left out, so sending and then reading your own history does
// not paint those messages as read. In a group, any other member's read
// produces a target for each author they actually saw.
func Targets(reader int64, messages []Message) []Target {
	if reader == 0 {
		return nil
	}
	best := make(map[int64]int64)
	order := make([]int64, 0, len(messages))
	for _, m := range messages {
		if m.SenderID == 0 || m.SenderID == reader || m.DialogMessageID == 0 {
			continue
		}
		prev, ok := best[m.SenderID]
		if !ok {
			order = append(order, m.SenderID)
		}
		if !ok || m.DialogMessageID > prev {
			best[m.SenderID] = m.DialogMessageID
		}
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]Target, 0, len(order))
	for _, id := range order {
		out = append(out, Target{SenderID: id, DialogMessageID: best[id]})
	}
	return out
}
