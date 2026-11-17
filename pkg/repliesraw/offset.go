// Package repliesraw is the internal history-read marker shared by the
// message service and contacts.blockFromReplies. It has no other behavior.
package repliesraw

import "math"

// OffsetDate on message.getHistoryMessages means: return the stored page,
// do not apply the expiry filter, and return database and context errors.
// Client history reads use offset_date >= 0.
const OffsetDate int32 = math.MinInt32
