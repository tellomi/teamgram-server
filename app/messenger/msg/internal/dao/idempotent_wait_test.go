package dao

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestIdempotentFollowerReusesLeaderResult(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		N int `json:"n"`
	}
	key := "idempotent#7@99"
	hold := make(chan struct{})
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		var got *result
		cached, err := doIdempotent(context.Background(), rds, key, &got, 5, 30, func(_ context.Context, v any) error {
			close(hold)
			time.Sleep(120 * time.Millisecond)
			*v.(**result) = &result{N: 7}
			return nil
		})
		if err != nil || cached {
			t.Errorf("leader cached=%v err=%v", cached, err)
		}
		if got == nil || got.N != 7 {
			t.Errorf("leader result = %#v", got)
		}
	}()

	select {
	case <-hold:
	case <-time.After(time.Second):
		t.Fatal("leader did not acquire the lock")
	}
	start := time.Now()
	var followed *result
	cached, err := doIdempotent(context.Background(), rds, key, &followed, 5, 30, func(context.Context, any) error {
		t.Error("follower ran the send")
		return nil
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if !cached {
		t.Fatal("follower missed the cached result")
	}
	if followed == nil || followed.N != 7 {
		t.Fatalf("follower result = %#v", followed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("follower waited %s; a 1s poll would still be waiting", elapsed)
	}
	<-leaderDone
}

func TestFanoutFailureIsNotCachedAndRetryPublishesOnce(t *testing.T) {
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	inserted := 0
	published := 0
	committed := false
	key := "idempotent#910442001@910442003"
	type result struct {
		OK bool `json:"ok"`
	}
	run := func() error {
		var got *result
		_, err := doIdempotent(context.Background(), rds, key, &got, 5, 30, func(_ context.Context, v any) error {
			if !committed {
				inserted++
				committed = true
				return errors.New("inbox fanout failed")
			}
			published++
			*v.(**result) = &result{OK: true}
			return nil
		})
		return err
	}
	if err := run(); err == nil {
		t.Fatal("a publish failure after the row commit must not be cached as success")
	}
	if inserted != 1 || published != 0 {
		t.Fatalf("after the failed publish inserted=%d published=%d", inserted, published)
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || published != 1 {
		t.Fatalf("retry inserted=%d published=%d, want the same row delivered once", inserted, published)
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if inserted != 1 || published != 1 {
		t.Fatalf("cached retry inserted=%d published=%d", inserted, published)
	}
}
