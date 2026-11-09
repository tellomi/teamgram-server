package location

import (
	"context"
	"sync"
	"time"
)

// Cache stores venue JSON and static-map bytes. A miss is (nil, false).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration)
	// Incr adds one to a counter and returns the new value. ttl applies when
	// the key is created. A backend error returns 0.
	Incr(ctx context.Context, key string, ttl time.Duration) int
}

type memEntry struct {
	val []byte
	exp time.Time
	n   int
}

type memoryCache struct {
	mu  sync.Mutex
	max int
	now func() time.Time
	m   map[string]memEntry
}

func newMemory(max int, now func() time.Time) *memoryCache {
	if max < 1 {
		max = 1
	}
	if now == nil {
		now = time.Now
	}
	return &memoryCache{max: max, now: now, m: map[string]memEntry{}}
}

func (c *memoryCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !e.exp.After(c.now()) {
		delete(c.m, key)
		return nil, false
	}
	return append([]byte(nil), e.val...), true
}

func (c *memoryCache) Set(_ context.Context, key string, val []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evict()
	c.m[key] = memEntry{val: append([]byte(nil), val...), exp: c.now().Add(ttl)}
}

func (c *memoryCache) Incr(_ context.Context, key string, ttl time.Duration) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !e.exp.After(c.now()) {
		e = memEntry{exp: c.now().Add(ttl)}
	}
	e.n++
	c.m[key] = e
	return e.n
}

func (c *memoryCache) evict() {
	now := c.now()
	if len(c.m) < c.max {
		for k, e := range c.m {
			if !e.exp.After(now) {
				delete(c.m, k)
			}
		}
		if len(c.m) < c.max {
			return
		}
	}
	for k := range c.m {
		delete(c.m, k)
		if len(c.m) < c.max/2 || len(c.m) == 0 {
			return
		}
	}
}
