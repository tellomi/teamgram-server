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

// Targets returns the other members whose messages reader has seen.
// The highest dialog message id wins for each sender. The reader's own
// messages are left out, so sending and then reading your own history does
// not paint those messages as read.
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
