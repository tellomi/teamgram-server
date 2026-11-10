package flood

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestRedisBucketMatchesMemory(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	store := NewRedisStore(rds)
	rule := Rule{Burst: 2, Rate: 0}
	now := time.Unix(1_700_000_000, 0)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		allowed, wait, err := store.Take(ctx, "flood:test:user:1", rule, now)
		if err != nil || !allowed || wait != 0 {
			t.Fatalf("take %d allowed %v wait %s err %v", i, allowed, wait, err)
		}
	}
	allowed, wait, err := store.Take(ctx, "flood:test:user:1", rule, now)
	if err != nil || allowed || wait < time.Second {
		t.Fatalf("allowed %v wait %s err %v", allowed, wait, err)
	}
	// A later now does not refill when the rate is zero.
	allowed, _, err = store.Take(ctx, "flood:test:user:1", rule, now.Add(time.Minute))
	if err != nil || allowed {
		t.Fatalf("refilled with rate 0: %v %v", allowed, err)
	}
	// A different key is a full bucket.
	allowed, _, err = store.Take(ctx, "flood:test:user:2", rule, now)
	if err != nil || !allowed {
		t.Fatalf("other key %v %v", allowed, err)
	}
}
