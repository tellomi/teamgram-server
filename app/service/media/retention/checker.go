package retention

import (
	"context"
	"sync"
	"time"
)

// ExpiryStore is the lookup upload.getFile needs.
type ExpiryStore interface {
	ExpiredAt(ctx context.Context, kind Kind, id int64) (int64, error)
}

// Checker answers "has this media expired?" for upload.getFile. Clients
// fetch a file in many parts, so answers are cached briefly. Lookup errors
// fail open: the download proceeds and dfs serves whatever exists.
type Checker struct {
	store ExpiryStore
	ttl   time.Duration
	max   int

	mu    sync.Mutex
	cache map[checkKey]checkVal
}

type checkKey struct {
	kind Kind
	id   int64
}

type checkVal struct {
	expired bool
	until   time.Time
}

func NewChecker(store ExpiryStore, ttl time.Duration) *Checker {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &Checker{store: store, ttl: ttl, max: 50000, cache: map[checkKey]checkVal{}}
}

func (c *Checker) Expired(ctx context.Context, kind Kind, id int64) bool {
	if c == nil || c.store == nil || id == 0 {
		return false
	}
	k := checkKey{kind, id}
	now := time.Now()
	c.mu.Lock()
	if v, ok := c.cache[k]; ok && now.Before(v.until) {
		c.mu.Unlock()
		return v.expired
	}
	c.mu.Unlock()
	at, err := c.store.ExpiredAt(ctx, kind, id)
	if err != nil {
		return false
	}
	c.mu.Lock()
	if len(c.cache) >= c.max {
		c.cache = map[checkKey]checkVal{}
	}
	c.cache[k] = checkVal{expired: at > 0, until: now.Add(c.ttl)}
	c.mu.Unlock()
	return at > 0
}
