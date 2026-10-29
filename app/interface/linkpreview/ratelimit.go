package linkpreview

import (
	"sync"
	"time"
)

type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	ticks   int
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(perMin, burst int) *limiter {
	if perMin <= 0 {
		perMin = 120
	}
	if burst <= 0 {
		burst = 40
	}
	return &limiter{
		rate:    float64(perMin) / 60,
		burst:   float64(burst),
		buckets: map[string]*bucket{},
	}
}

func (l *limiter) allow(key string, now time.Time) bool {
	if key == "" {
		key = "-"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ticks++
	if l.ticks%256 == 0 {
		l.gc(now)
	}
	b := l.buckets[key]
	if b == nil {
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *limiter) gc(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.last) > 10*time.Minute {
			delete(l.buckets, k)
		}
	}
}
