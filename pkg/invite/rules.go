// Package invite is the shared validation for messages.exportChatInvite.
// Basic groups and channels use the same limits the official clients send.
package invite

import (
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

// TitleMax is the invite title length official clients accept.
const TitleMax = 32

// Export is one messages.exportChatInvite call.
// A zero Expire means no expiry. UsageSet and TitleSet are the TL flags:
// a present usage_limit of 0 is invalid, while an absent one means unlimited.
type Export struct {
	Legacy        bool
	RequestNeeded bool
	Expire        int64
	Usage         int32
	UsageSet      bool
	Title         string
	TitleSet      bool
	Now           int64
}

// Validate applies the official errors for a new invite link.
// legacy_revoke_permanent regenerates the primary link and cannot carry
// a title, an expiry, a usage limit, or an approval requirement.
func (e Export) Validate() error {
	now := e.Now
	if now == 0 {
		now = time.Now().Unix()
	}
	if e.Legacy && (e.Expire != 0 || e.UsageSet || e.TitleSet || e.RequestNeeded) {
		return mtproto.ErrChatInvitePermanent
	}
	if e.Expire != 0 && e.Expire <= now {
		return mtproto.ErrExpireDateInvalid
	}
	if e.UsageSet && e.Usage <= 0 {
		return mtproto.ErrUsageLimitInvalid
	}
	if e.TitleSet {
		n := utf8.RuneCountInString(e.Title)
		if n == 0 || n > TitleMax {
			return mtproto.ErrTitleInvalid
		}
	}
	return nil
}

// TitleOK reports whether an edited title is within the official limit.
// An empty title clears the name.
func TitleOK(title string) bool {
	return utf8.RuneCountInString(title) <= TitleMax
}
