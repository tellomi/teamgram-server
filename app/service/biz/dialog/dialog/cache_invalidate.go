package dialog

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlc"

	"github.com/zeromicro/go-zero/core/logx"
)

// DefaultCacheInvalidateDelay is the wait before the second DelCache.
// CachedConn.Exec deletes the key after the SQL write, but an in-flight
// QueryRow / QueryRows (TakeCtx) can still SetEx the object it loaded
// before that delete. A delayed second delete drops that stale backfill.
// Marmota already uses a 5s window (cacheSafeGapBetweenIndexAndPrimary)
// for index-vs-primary cache writes; one second covers a typical in-flight
// getDialogs without leaving pinned/notify/unread stale for long.
const DefaultCacheInvalidateDelay = time.Second

// ScheduleDelCache deletes keys after delay. A delay <= 0 uses
// DefaultCacheInvalidateDelay. The returned channel is closed after the
// second delete attempt (or immediately when there are no keys).
func ScheduleDelCache(conn sqlc.CachedConn, delay time.Duration, keys ...string) <-chan struct{} {
	done := make(chan struct{})
	if len(keys) == 0 {
		close(done)
		return done
	}
	if delay <= 0 {
		delay = DefaultCacheInvalidateDelay
	}
	copied := append([]string(nil), keys...)
	go func() {
		defer close(done)
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := conn.DelCache(ctx, copied...); err != nil {
			logx.Errorf("dialog: delayed DelCache keys=%v err=%v", copied, err)
		}
	}()
	return done
}

// ExecAndInvalidate runs exec, deletes the keys when it succeeds (the
// original CachedConn.Exec behavior), and schedules a second delete so a
// concurrent cache backfill cannot stick. The channel closes after that
// second delete. On exec error the keys are left alone and the channel
// is already closed.
func ExecAndInvalidate(ctx context.Context, conn sqlc.CachedConn, delay time.Duration, exec sqlc.ExecFn, keys ...string) (lastInsertId, rowsAffected int64, done <-chan struct{}, err error) {
	closed := make(chan struct{})
	close(closed)
	lastInsertId, rowsAffected, err = conn.Exec(ctx, exec, keys...)
	if err != nil {
		return lastInsertId, rowsAffected, closed, err
	}
	return lastInsertId, rowsAffected, ScheduleDelCache(conn, delay, keys...), nil
}
