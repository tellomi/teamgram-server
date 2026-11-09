package location

import (
	"sync"
	"time"
)

const breakerOpen = 10 * time.Minute

// gate is the per-user token bucket, the global QPS cap, the daily budget,
// and the circuit breaker. A closed gate means "serve cache or an empty list".
type gate struct {
	mu         sync.Mutex
	userRate   float64
	userBurst  float64
	users      map[int64]*bucket
	global     *bucket
	budget     int
	used       int
	day        string
	openUntil  time.Time
	now        func() time.Time
	count      func(day string) int
	tripRemote func(until time.Time)
	openRemote func() (time.Time, bool)
}

type bucket struct {
	tokens  float64
	updated time.Time
	rate    float64
	burst   float64
}

func (b *bucket) allow(now time.Time) bool {
	if b.updated.IsZero() {
		b.tokens = b.burst
		b.updated = now
	} else {
		elapsed := now.Sub(b.updated).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * b.rate
			if b.tokens > b.burst {
				b.tokens = b.burst
			}
			b.updated = now
		}
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (g *gate) allow(user int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if !g.openUntil.IsZero() && now.Before(g.openUntil) {
		return false
	}
	if g.openRemote != nil {
		if until, ok := g.openRemote(); ok && now.Before(until) {
			g.openUntil = until
			return false
		}
	}
	if g.users == nil {
		g.users = map[int64]*bucket{}
	}
	b := g.users[user]
	if b == nil {
		b = &bucket{rate: g.userRate, burst: g.userBurst}
		g.users[user] = b
	}
	if !b.allow(now) || !g.global.allow(now) {
		return false
	}
	day := shanghaiDay(now)
	if g.day != day {
		g.day = day
		g.used = 0
	}
	n := g.used + 1
	if g.count != nil {
		if remote := g.count(day); remote > n {
			n = remote
		}
	}
	if g.budget > 0 && n > g.budget {
		g.trip(now)
		return false
	}
	g.used = n
	return true
}

func (g *gate) trip(now time.Time) {
	until := now.Add(breakerOpen)
	if g.openUntil.Before(until) {
		g.openUntil = until
	}
	if g.tripRemote != nil {
		g.tripRemote(until)
	}
}

func (g *gate) open(now time.Time) {
	g.mu.Lock()
	g.trip(now)
	g.mu.Unlock()
}

func shanghaiDay(now time.Time) string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return now.In(loc).Format("20060102")
}
