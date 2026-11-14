package channel

import (
	"context"
	"strings"
)

// LookupPosterName returns the sender's first and last name. The msg service
// installs it from the user record. A client-supplied post_author is never used.
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

// signatureText is the channel post signature. channels.editAdmin rank is the
// admin's custom title and replaces the name when it is set. Otherwise the
// signature is the name official clients show (Desktop NewMessagePostAuthor).
// An empty name leaves post_author unset. FwdFrom.post_author is not touched.
func (s *Service) signatureText(ctx context.Context, channelID, sender int64) string {
	if s != nil && s.store != nil && channelID != 0 && sender != 0 {
		p, err := s.store.GetParticipant(ctx, channelID, sender)
		if err == nil && p != nil && p.IsAdmin() {
			if rank := strings.TrimSpace(p.Rank); rank != "" {
				return rank
			}
		}
	}
	return posterName(ctx, sender)
}
