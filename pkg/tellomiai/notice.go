package tellomiai

import (
	"sync"
	"time"
)

const (
	noticeMinute = time.Minute
	noticeDeny   = 10 * time.Minute
	noticeHour   = time.Hour
)

// noticeLog drops repeated fixed replies. The first notice in a window is sent.
type noticeLog struct {
	mu sync.Mutex
	at map[string]time.Time
}

// admit reports whether this fixed reply should be delivered.
// Success replies (empty kind) are always delivered. Repeated is handled
// by the caller before admit.
func (n *noticeLog) admit(kind string, user int64, chat string, group bool, now time.Time) bool {
	scope, window := noticeScope(kind, group)
	if window <= 0 {
		return true
	}
	key := scope + ":" + kind + ":" + itoa(user)
	if scope == "c" {
		key = "c:" + kind + ":" + chat
	} else if scope == "g" {
		key = "g:" + kind + ":" + itoa(user) + ":" + chat
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.at == nil {
		n.at = map[string]time.Time{}
	}
	if prev, ok := n.at[key]; ok && now.Sub(prev) < window {
		return false
	}
	n.at[key] = now
	return true
}

func noticeScope(kind string, group bool) (scope string, window time.Duration) {
	switch kind {
	case KindAllow:
		// Not-allowed users can mention on every message. One notice is enough.
		if group {
			return "g", noticeDeny
		}
		return "u", noticeMinute
	case KindDaily, KindBudget:
		// Already over quota: at most one notice an hour, not one per message.
		return "u", noticeHour
	case KindMinute:
		return "u", noticeMinute
	case KindChat:
		// The cap used to be silent so a busy group would not fill with notices.
		return "c", noticeMinute
	default:
		// Disabled, empty input, moderation, and upstream errors are delivered
		// each time. They are not the repeating quota notices.
		return "", 0
	}
}
