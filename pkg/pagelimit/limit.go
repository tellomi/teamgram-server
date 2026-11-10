// Package pagelimit is the page size official clients send for history.
//
// messages.getHistory, messages.search, messages.getReplies, and the
// channel message reads that share that paging accept a limit up to 100.
// https://core.telegram.org/api/offsets
package pagelimit

// Max is the largest page those methods return.
const Max int32 = 100

// Cap leaves a non-positive limit unchanged and clamps anything above Max.
// A client that asked for 0 still gets the handler's own empty page.
func Cap(limit int32) int32 {
	if limit > Max {
		return Max
	}
	return limit
}

// OrDefault uses def when limit is missing and clamps the rest to Max.
// def itself is not clamped; callers pass a value inside Max.
func OrDefault(limit, def int32) int32 {
	if limit <= 0 {
		return def
	}
	if limit > Max {
		return Max
	}
	return limit
}
