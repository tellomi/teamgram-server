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
// fetch a file in many parts, so an expired answer is cached briefly.
// A live answer is not cached: expired_at only moves from 0 to a timestamp,
// and a cached "still live" result would keep upload.getFile serving the
// bytes after the ttl sweeper had already marked the row. Lookup errors
// fail open: the download proceeds and dfs serves whatever exists.
// upload.getFile consults this check before the dfs chunk cache, so a live
// re-read is what makes the file undownloadable. The sweeper runs in another
// process and cannot drop this map; Forget only reaches checkers in this one.
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
	c := &Checker{store: store, ttl: ttl, max: 50000, cache: map[checkKey]checkVal{}}
	registerChecker(c)
	return c
}

var (
	regMu    sync.Mutex
	checkers []*Checker
)

func registerChecker(c *Checker) {
	if c == nil {
		return
	}
	regMu.Lock()
	checkers = append(checkers, c)
	regMu.Unlock()
}

// Forget drops one id from every checker in this process. The files service
// and the ttl sweeper do not share memory, so upload.getFile also refuses to
// cache a live answer.
func Forget(kind Kind, id int64) {
	if id == 0 {
		return
	}
	regMu.Lock()
	list := append([]*Checker(nil), checkers...)
	regMu.Unlock()
	for _, c := range list {
		c.Forget(kind, id)
	}
}

// Forget drops one cached answer.
func (c *Checker) Forget(kind Kind, id int64) {
	if c == nil || id == 0 {
		return
	}
	c.mu.Lock()
	delete(c.cache, checkKey{kind, id})
	c.mu.Unlock()
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
	if at <= 0 {
		// Do not remember "still live". The next getFile must see a
		// concurrent expired_at write.
		c.Forget(kind, id)
		return false
	}
	c.mu.Lock()
	if len(c.cache) >= c.max {
		c.cache = map[checkKey]checkVal{}
	}
	c.cache[k] = checkVal{expired: true, until: now.Add(c.ttl)}
	c.mu.Unlock()
	return true
}
