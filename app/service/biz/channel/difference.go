package channel

import (
	"context"
	"sort"
	"sync/atomic"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// Official updates.getChannelDifference: ordinary users pass 10–100.
	// The method maximum is 100000, which bots may use.
	// https://core.telegram.org/method/updates.getChannelDifference
	channelDifferenceLimitUser int32 = 100
	channelDifferenceLimitBot  int32 = 100000
)

// channelDifferenceTooLongTotal counts updates.channelDifferenceTooLong
// replies. It does not include message text.
var channelDifferenceTooLongTotal atomic.Int64

// ChannelDifferenceTooLongTotal is how many channelDifferenceTooLong
// replies this process has built.
func ChannelDifferenceTooLongTotal() int64 {
	return channelDifferenceTooLongTotal.Load()
}

// clampChannelDifferenceLimit applies the official page size.
// A missing limit becomes 100. Users cannot ask for more than 100.
// Bots can ask for up to 100000.
func clampChannelDifferenceLimit(limit int32, bot bool) int32 {
	max := channelDifferenceLimitUser
	if bot {
		max = channelDifferenceLimitBot
	}
	if limit <= 0 {
		return channelDifferenceLimitUser
	}
	if limit > max {
		return max
	}
	return limit
}

// ptsContinues reports that e still covers the client's cursor.
// The event occupies (pts-pts_count, pts]. A hole is a start strictly
// after the cursor. pts_count above 1 is one event, not a hole.
func ptsContinues(cursor int32, e *PtsEvent) bool {
	if e == nil || e.Pts <= cursor {
		return false
	}
	count := e.PtsCount
	if count < 1 {
		count = 1
	}
	if count > e.Pts {
		return true
	}
	return e.Pts-count <= cursor
}

// continuousPage is the next channelDifference page.
// more means the client must call again: another event is waiting, or a
// hole begins after this page. An empty page means the log no longer
// contains the cursor, which is channelDifferenceTooLong.
func continuousPage(events []*PtsEvent, cursor int32, limit int) (page []*PtsEvent, more bool) {
	if limit <= 0 {
		limit = int(channelDifferenceLimitUser)
	}
	for _, e := range events {
		if !ptsContinues(cursor, e) {
			return page, len(page) > 0
		}
		if len(page) >= limit {
			return page, true
		}
		page = append(page, e)
		cursor = e.Pts
	}
	return page, false
}

// latestChannelMessages is the newest non-deleted messages, up to limit.
// channelDifferenceTooLong carries this snapshot. getHistory's own cap
// stays in selectHistory.
func latestChannelMessages(all []*StoredMessage, limit int32) []*StoredMessage {
	if limit <= 0 {
		limit = channelDifferenceLimitUser
	}
	live := make([]*StoredMessage, 0, len(all))
	for _, m := range all {
		if m == nil || m.Deleted {
			continue
		}
		live = append(live, m)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].ID > live[j].ID })
	if int32(len(live)) > limit {
		live = live[:limit]
	}
	return live
}

func noteChannelDifferenceTooLong(ctx context.Context, channelID, userID int64, clientPts, serverPts int32, reason string) {
	n := channelDifferenceTooLongTotal.Add(1)
	logx.WithContext(ctx).Infof("updates.getChannelDifference tooLong channel_id=%d user_id=%d client_pts=%d server_pts=%d reason=%s total=%d",
		channelID, userID, clientPts, serverPts, reason, n)
}
