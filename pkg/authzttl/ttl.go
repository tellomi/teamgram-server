// Package authzttl is the official account.setAuthorizationTTL rule.
// A session that has been inactive for more than the stored number of days
// is expired. Day 0 and a missing row use the server default of 180 days,
// which is the users.authorization_ttl_days column default.
package authzttl

// DefaultDays is the inactivity window when the account has not stored one.
const DefaultDays int32 = 180

// Day is one TTL day in seconds. The official parameter is a whole number of days.
const Day int64 = 24 * 60 * 60

// TouchGap is how often a live session refreshes auth_users.date_active.
// Ping and RPC both count as activity. Writing the row on every ping would
// keep MySQL busy; an hour is still far shorter than the shortest official
// window (7 days).
const TouchGap int64 = 60 * 60

// Days returns the window to enforce. Zero and negative values are the default.
func Days(stored int32) int32 {
	if stored <= 0 {
		return DefaultDays
	}
	return stored
}

// Expired reports that lastActive is outside the inactivity window.
// A missing or future lastActive is not expired: those rows are not evidence
// the session has been idle, and treating them as idle would log the account out.
func Expired(lastActive, now int64, ttlDays int32) bool {
	if lastActive <= 0 || now <= lastActive {
		return false
	}
	return now-lastActive >= int64(Days(ttlDays))*Day
}
