// Package postsearch is the free daily quota for channels.searchPosts.
//
// Telegram clients read channels.checkSearchPostsFlood before global post
// search. A zero total_daily makes iOS draw "已达上限，每天 0 次 / 花费⭐0".
// Tellomi search is free: query_is_free stays true and stars_amount stays 0.
// The remaining count is a Redis counter that resets at midnight in
// Asia/Shanghai. TELLOMI_POST_SEARCH_DAILY sets the cap (default 100).
package postsearch

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// shanghai is China Standard Time. It does not observe daylight saving.
var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

const (
	defaultDaily = int32(100)
	envDaily     = "TELLOMI_POST_SEARCH_DAILY"
	keyPrefix    = "tellomi:post_search:"
)

// KV is the Redis subset that keeps the per-user counter.
// IncrCtx is atomic across processes. The key includes the Shanghai date,
// and ExpireCtx drops it after the day ends.
type KV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	IncrCtx(ctx context.Context, key string) (int64, error)
	ExpireCtx(ctx context.Context, key string, seconds int) error
}

// Book is the process-wide post-search quota.
type Book struct {
	kv  KV
	now func() time.Time
}

// New uses Redis when conf is set. An empty config keeps the counter in
// memory so a process without KV still returns a real quota.
func New(conf gokv.KvConf) *Book {
	var store KV
	if len(conf) == 0 {
		store = newMem()
	} else {
		store = gokv.NewStore(conf)
	}
	return &Book{kv: store, now: time.Now}
}

var (
	mu     sync.RWMutex
	shared = New(nil)
)

// Shared is the quota checkSearchPostsFlood reads and searchPosts spends.
func Shared() *Book {
	mu.RLock()
	defer mu.RUnlock()
	return shared
}

// Install replaces the process-wide book. Nil installs an in-memory book.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	mu.Lock()
	shared = b
	mu.Unlock()
}

// Peek reports the quota without spending a search.
func (b *Book) Peek(ctx context.Context, userID int64) *mtproto.SearchPostsFlood {
	return b.flood(b.used(ctx, userID))
}

// Spend counts one channels.searchPosts call and returns the quota after it.
// A user who is already at zero stays at zero. The search itself stays free.
func (b *Book) Spend(ctx context.Context, userID int64) *mtproto.SearchPostsFlood {
	if b == nil || b.kv == nil || userID == 0 {
		return freeFlood(dailyCap(), dailyCap())
	}
	total := dailyCap()
	key := b.dayKey(userID)
	used := b.used(ctx, userID)
	if used < total {
		n, err := b.kv.IncrCtx(ctx, key)
		if err != nil {
			return freeFlood(total, total-used)
		}
		_ = b.kv.ExpireCtx(ctx, key, b.secondsUntilNextDay()+3600)
		used = int32(n)
		if used < 0 {
			used = 0
		}
	}
	return b.flood(used)
}

func (b *Book) used(ctx context.Context, userID int64) int32 {
	if b == nil || b.kv == nil || userID == 0 {
		return 0
	}
	raw, err := b.kv.GetCtx(ctx, b.dayKey(userID))
	if err != nil || raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

func (b *Book) flood(used int32) *mtproto.SearchPostsFlood {
	total := dailyCap()
	remains := total - used
	if remains < 0 {
		remains = 0
	}
	return freeFlood(total, remains, b.wait(remains))
}

func (b *Book) wait(remains int32) int32 {
	if remains > 0 || b == nil {
		return 0
	}
	now := b.now().In(shanghai)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, shanghai)
	return int32(next.Unix())
}

func (b *Book) dayKey(userID int64) string {
	day := b.now().In(shanghai).Format("20060102")
	return keyPrefix + strconv.FormatInt(userID, 10) + ":" + day
}

func (b *Book) secondsUntilNextDay() int {
	now := b.now().In(shanghai)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, shanghai)
	sec := int(next.Sub(now).Seconds())
	if sec < 1 {
		return 1
	}
	return sec
}

// freeFlood is the object iOS reads. stars_amount is always zero.
// waitTill is omitted while remains is above zero.
func freeFlood(total, remains int32, waitTill ...int32) *mtproto.SearchPostsFlood {
	f := mtproto.MakeTLSearchPostsFlood(&mtproto.SearchPostsFlood{
		QueryIsFree: true,
		TotalDaily:  total,
		Remains:     remains,
		StarsAmount: 0,
	}).To_SearchPostsFlood()
	if len(waitTill) > 0 && waitTill[0] > 0 && remains <= 0 {
		f.WaitTill = &wrapperspb.Int32Value{Value: waitTill[0]}
	}
	return f
}

func dailyCap() int32 {
	raw := os.Getenv(envDaily)
	if raw == "" {
		return defaultDaily
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultDaily
	}
	if n > 100000 {
		return 100000
	}
	return int32(n)
}

type memKV struct {
	mu sync.Mutex
	m  map[string]string
}

func newMem() *memKV {
	return &memKV{m: map[string]string{}}
}

func (m *memKV) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[key], nil
}

func (m *memKV) IncrCtx(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 1
	if v := m.m[key]; v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed + 1
		}
	}
	m.m[key] = strconv.Itoa(n)
	return int64(n), nil
}

func (m *memKV) ExpireCtx(context.Context, string, int) error { return nil }
