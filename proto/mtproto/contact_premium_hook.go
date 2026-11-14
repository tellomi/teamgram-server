package mtproto

import "sync/atomic"

// contactRequirePremiumFn reports whether userID has turned on
// globalPrivacySettings.new_noncontact_peers_require_premium.
// The BFF installs it. Until then the flag stays unset.
//
// https://core.telegram.org/api/privacy
// user.contact_require_premium is that setting, shown to other users.
// It is not the fully checked "this viewer cannot write" bit. That bit is
// userFull.contact_require_premium and users.getRequirementsToContact.
var contactRequirePremiumFn atomic.Value // func(int64) bool

var invalidateContactRequirePremiumFn atomic.Value // func(int64)

// SetContactRequirePremiumFunc installs the lookup used when a user object
// is rendered for someone else. nil clears it.
func SetContactRequirePremiumFunc(fn func(int64) bool) {
	if fn == nil {
		contactRequirePremiumFn.Store(func(int64) bool { return false })
		return
	}
	contactRequirePremiumFn.Store(fn)
}

// ContactRequirePremium is the raw new_noncontact_peers_require_premium
// setting of userID. Self and deleted cards do not call this.
func ContactRequirePremium(userID int64) bool {
	if userID == 0 {
		return false
	}
	fn, _ := contactRequirePremiumFn.Load().(func(int64) bool)
	if fn == nil {
		return false
	}
	return fn(userID)
}

// SetInvalidateContactRequirePremiumFunc installs the cache drop used after
// account.setGlobalPrivacySettings writes the flag.
func SetInvalidateContactRequirePremiumFunc(fn func(int64)) {
	if fn == nil {
		invalidateContactRequirePremiumFn.Store(func(int64) {})
		return
	}
	invalidateContactRequirePremiumFn.Store(fn)
}

// InvalidateContactRequirePremium drops a cached copy of the flag.
func InvalidateContactRequirePremium(userID int64) {
	fn, _ := invalidateContactRequirePremiumFn.Load().(func(int64))
	if fn == nil || userID == 0 {
		return
	}
	fn(userID)
}
