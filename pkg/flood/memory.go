package flood

import (
	"context"
	"sync"
	"time"
)

type bucketState struct {
	tokens float64
	ts     time.Time
	ready  bool
}

// Memory is a process-local token bucket. Tests use it. Production uses Redis
// so a second session process sees the same counters.
type Memory struct {
	mu sync.Mutex
	m  map[string]bucketState
}

func NewMemory() *Memory {
	return &Memory{m: map[string]bucketState{}}
}

func (m *Memory) Take(_ context.Context, key string, rule Rule, now time.Time) (bool, time.Duration, error) {
	if m == nil {
		return true, 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.m[key]
	tokens := st.tokens
	ts := st.ts
	if !st.ready {
		tokens = float64(rule.Burst)
		ts = now
	}
	allowed, wait, next := takeToken(tokens, ts, now, rule)
	m.m[key] = bucketState{tokens: next, ts: now, ready: true}
	return allowed, wait, nil
}

func takeToken(tokens float64, ts, now time.Time, rule Rule) (allowed bool, wait time.Duration, next float64) {
	elapsed := now.Sub(ts).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	tokens += elapsed * rule.Rate
	if tokens > float64(rule.Burst) {
		tokens = float64(rule.Burst)
	}
	if tokens >= 1 {
		return true, 0, tokens - 1
	}
	if rule.Rate <= 0 {
		return false, time.Hour, tokens
	}
	missing := 1 - tokens
	secs := missing / rule.Rate
	return false, time.Duration(secs * float64(time.Second)), tokens
}
