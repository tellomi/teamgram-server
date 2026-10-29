package mtproto

import "sync/atomic"

// membershipFunc reports whether a stored subscription is an active Premium
// membership. expireUnix 0 means the flag has no end date. The process may
// replace this with a config-backed policy; the default is the stored row.
type membershipFunc func(stored bool, expireUnix, now int64) bool

var membership atomic.Value

func init() {
	membership.Store(membershipFunc(storedMembership))
}

func storedMembership(stored bool, expireUnix, now int64) bool {
	if !stored {
		return false
	}
	if expireUnix == 0 {
		return true
	}
	return now < expireUnix
}

// SetMembershipFunc installs the process-wide Premium membership policy.
// A nil function restores the stored-subscription check.
func SetMembershipFunc(fn func(stored bool, expireUnix, now int64) bool) {
	if fn == nil {
		membership.Store(membershipFunc(storedMembership))
		return
	}
	membership.Store(membershipFunc(fn))
}

func currentMembership() membershipFunc {
	fn, _ := membership.Load().(membershipFunc)
	if fn == nil {
		return storedMembership
	}
	return fn
}
