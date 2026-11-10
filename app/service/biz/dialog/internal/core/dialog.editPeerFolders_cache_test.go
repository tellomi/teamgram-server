package core

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// CachedConn.Exec drops its keys when the callback returns a nil error.
// editPeerFolders reports (0, 0, nil). The reliability branch reported
// (0, 1, nil). The row count is not part of the eviction decision.
func TestCachedConnExecDropsFolderKeysOnEitherSuccessCount(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	conn := sqlc.NewNodeConn(nil, rds)
	ctx := context.Background()

	for _, rows := range []int64{0, 1} {
		key := dialogCacheKeys(10, []int64{20})[0] + "/rows"
		if rows == 1 {
			key += "-one"
		}
		if err := conn.SetCache(ctx, key, "stale-folder"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := conn.Exec(ctx, func(context.Context, *sqlx.DB) (int64, int64, error) {
			return 0, rows, nil
		}, key); err != nil {
			t.Fatal(err)
		}
		var got string
		if err := conn.GetCache(ctx, key, &got); err == nil {
			t.Fatalf("rowsAffected %d left %q in %s", rows, got, key)
		}
	}

	key := dialogCacheKeys(10, []int64{20})[0] + "/keep"
	if err := conn.SetCache(ctx, key, "stale-folder"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.Exec(ctx, func(context.Context, *sqlx.DB) (int64, int64, error) {
		return 0, 0, errors.New("folder write failed")
	}, key); err == nil {
		t.Fatal("expected the callback error")
	}
	var got string
	if err := conn.GetCache(ctx, key, &got); err != nil || got != "stale-folder" {
		t.Fatalf("failed write evicted or changed the cache: %q %v", got, err)
	}
}
