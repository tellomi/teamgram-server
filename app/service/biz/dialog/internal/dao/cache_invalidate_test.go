package dao

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// TestDelayedDoubleDeleteDropsStaleBackfill simulates the setHistoryTTL
// race: an in-flight getDialogs already loaded the pre-write object, the
// writer deletes the key, then TakeCtx SetExs that stale object. The
// delayed second delete must drop it so pinned/notify/unread (and ttl)
// are not served from Redis for days.
func TestDelayedDoubleDeleteDropsStaleBackfill(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	conn := sqlc.NewNodeConn(nil, rds)
	d := &Dao{CachedConn: conn, InvalidateDelay: 40 * time.Millisecond}
	ctx := context.Background()
	key := dialog.GetDialogCacheKeyByPeer(10, mtproto.PEER_USER, 20)
	const stale = "ttl=0 unread=5"

	if err := conn.SetCache(ctx, key, stale); err != nil {
		t.Fatal(err)
	}

	// In-flight getDialogs: query already ran against the old row. It
	// waits until after the writer's first DelCache, then SetExs.
	startBackfill := make(chan struct{})
	backfilled := make(chan struct{})
	go func() {
		<-startBackfill
		if err := conn.SetCache(ctx, key, stale); err != nil {
			t.Errorf("stale SetEx: %v", err)
		}
		close(backfilled)
	}()

	_, _, done, err := dialog.ExecAndInvalidate(ctx, d.CachedConn, d.InvalidateDelay, func(context.Context, *sqlx.DB) (int64, int64, error) {
		return 0, 1, nil
	}, key)
	if err != nil {
		t.Fatal(err)
	}

	var got string
	if err := conn.GetCache(ctx, key, &got); err == nil {
		t.Fatalf("first delete left %q", got)
	}

	close(startBackfill)
	<-backfilled
	if err := conn.GetCache(ctx, key, &got); err != nil || got != stale {
		t.Fatalf("expected stale backfill before second delete, got %q err=%v", got, err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second delete did not finish")
	}
	if err := conn.GetCache(ctx, key, &got); err == nil {
		t.Fatalf("second delete left %q", got)
	}
}

func TestExecInvalidateKeepsCacheWhenWriteFails(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	conn := sqlc.NewNodeConn(nil, rds)
	d := &Dao{CachedConn: conn, InvalidateDelay: 20 * time.Millisecond}
	ctx := context.Background()
	key := dialog.GetDialogCacheKeyByPeer(10, mtproto.PEER_USER, 20)
	if err := conn.SetCache(ctx, key, "keep"); err != nil {
		t.Fatal(err)
	}
	_, _, err = d.ExecInvalidate(ctx, func(context.Context, *sqlx.DB) (int64, int64, error) {
		return 0, 0, errors.New("ttl write failed")
	}, key)
	if err == nil {
		t.Fatal("expected write error")
	}
	var got string
	if err := conn.GetCache(ctx, key, &got); err != nil || got != "keep" {
		t.Fatalf("failed write evicted the cache: %q %v", got, err)
	}
}
