package dao

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/idempotent"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// idempotentPoll is how often a resend looks for the leader's cached result.
// The upstream helper polls once a second, so a client retry held the
// per-auth-key RPC loop for up to a second after the original send finished.
const idempotentPoll = 25 * time.Millisecond

// doIdempotent matches marmota's DoIdempotent lock and cache keys, and polls
// the cache at idempotentPoll instead of once a second.
func doIdempotent(ctx context.Context, store *redis.Redis, key string, v any, seconds, expired int, fn func(context.Context, any) error) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lock := idempotent.NewIdempotent(store, key)
	cached, err := lock.TryGetCacheValue(ctx, v)
	if err != nil {
		return false, err
	}
	if cached {
		return true, nil
	}

	ok, err := lock.Lock(ctx, seconds)
	if err != nil {
		return false, err
	}
	if ok {
		defer lock.Unlock(ctx)
		if err = fn(ctx, v); err != nil {
			return false, err
		}
		if err = lock.SetCacheValue(ctx, v, expired); err != nil {
			return false, err
		}
		return false, nil
	}

	wait := time.Duration(seconds) * time.Second
	if seconds < 1 {
		wait = time.Second
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(idempotentPoll)
	defer ticker.Stop()
	for {
		cached, err = lock.TryGetCacheValue(ctx, v)
		if err != nil {
			return false, err
		}
		if cached {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, idempotent.ErrorCacheEmpty
		case <-ticker.C:
		}
	}
}
