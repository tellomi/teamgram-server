package channel

import (
	"context"
	"strings"
)

// LookupPosterName returns the display name stored in message.post_author
// when a channel has signatures enabled. The msg service installs it.
var LookupPosterName func(ctx context.Context, userID int64) string

// FormatPosterName joins a user's first and last name the way channel
// signatures show them.
func FormatPosterName(first, last string) string {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	switch {
	case first == "" && last == "":
		return ""
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}

func posterName(ctx context.Context, userID int64) string {
	if LookupPosterName == nil || userID == 0 {
		return ""
	}
	return strings.TrimSpace(LookupPosterName(ctx, userID))
}
