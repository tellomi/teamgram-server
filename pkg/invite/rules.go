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

// UsageMax is the official upper bound for usage_limit (1 through 99999).
const UsageMax int32 = 99999

// Export is one messages.exportChatInvite call.
// A zero Expire means no expiry. UsageSet and TitleSet are the TL flags.
// Android LinkEditActivity sets the usage_limit flag and sends 0 for no limit.
// That 0 is unlimited: store it as 0 and omit the field on chatInviteExported.
// An absent flag is the same. A negative value, or one above UsageMax, is
// USAGE_LIMIT_INVALID.
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
	if _, err := NormalizeUsage(e.UsageSet, e.Usage); err != nil {
		return err
	}
	if e.TitleSet {
		n := utf8.RuneCountInString(e.Title)
		if n == 0 || n > TitleMax {
			return mtproto.ErrTitleInvalid
		}
	}
	return nil
}

// StoredUsage is the usage_limit written after Validate has passed.
// 0 means unlimited and is omitted from chatInviteExported.
func (e Export) StoredUsage() int32 {
	stored, err := NormalizeUsage(e.UsageSet, e.Usage)
	if err != nil {
		return 0
	}
	return stored
}

// NormalizeUsage maps a client usage_limit onto the stored value.
// The flag may be set with 0. Official Telegram treats that as no limit,
// the same as an absent flag. The stored 0 is what MakeFlagsInt32 omits.
// Negative values and values above UsageMax are USAGE_LIMIT_INVALID.
func NormalizeUsage(set bool, value int32) (int32, error) {
	if !set || value == 0 {
		return 0, nil
	}
	if value < 0 || value > UsageMax {
		return 0, mtproto.ErrUsageLimitInvalid
	}
	return value, nil
}

// UsageExhausted reports that a limited link has no joins left.
// A stored limit of 0 is unlimited, including a usage_limit the client sent as 0.
func UsageExhausted(limit int32, used int) bool {
	return limit > 0 && used >= int(limit)
}

// NotStarted reports a link whose start_date is still in the future.
// A zero start_date means the link is usable immediately.
func NotStarted(start, now int64) bool {
	return start > 0 && now < start
}

// TitleOK reports whether an edited title is within the official limit.
// An empty title clears the name.
func TitleOK(title string) bool {
	return utf8.RuneCountInString(title) <= TitleMax
}
