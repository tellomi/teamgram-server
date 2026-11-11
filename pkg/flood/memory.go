package flood

import (
	"context"
	"strings"
	"sync"
	"time"
)

type bucketState struct {
	tokens float64
	ts     time.Time
	ready  bool
}

const (
	memoryMaxKeys  = 4096
	memoryTTL      = 24 * time.Hour
	memoryOverflow = "flood:local:overflow"
	// memoryGate is the conservative total for every overflow bucket. Its
	// rate does not change with the caller, so a fast method cannot refill
	// a slow one.
	memoryGate = memoryOverflow + ":gate"
)

// gateRule is one action per 100s once the subject map is full. It is not
// the caller's burst.
var gateRule = Rule{Burst: 1, Rate: 0.01}

// Memory is a process-local token bucket. Tests use it. Production uses it
// only while Redis is down, and only for high-cost buckets. The map expires
// entries after memoryTTL and stops inserting once it is full. A full map
// shares one overflow bucket instead of evicting a limited subject back to
// a fresh burst.
type Memory struct {
	mu      sync.Mutex
	m       map[string]bucketState
	maxKeys int
	ttl     time.Duration
}

func NewMemory() *Memory {
	return &Memory{m: map[string]bucketState{}, maxKeys: memoryMaxKeys, ttl: memoryTTL}
}

func (m *Memory) Len() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.m)
}

func (m *Memory) Take(ctx context.Context, key string, rule Rule, now time.Time) (bool, time.Duration, error) {
	allowed, wait, _, err := m.take(ctx, key, rule, now)
	return allowed, wait, err
}

// take reports fresh when this call inserted a subject key that a later
// rejection in the same request may drop. The overflow key is never fresh.
func (m *Memory) take(_ context.Context, key string, rule Rule, now time.Time) (bool, time.Duration, bool, error) {
	if m == nil {
		return true, 0, false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	overflow := false
	if _, ok := m.m[key]; !ok && m.subjectCount() >= m.maxKeys {
		key = overflowKey(key)
		overflow = true
	}
	allowed, wait, fresh := m.takeKey(key, rule, now)
	if !overflow || !allowed {
		return allowed, wait, fresh, nil
	}
	// The per-bucket overflow token is already spent. The shared gate uses
	// its own rule, so this spend cannot be refilled by another method.
	gateOK, gateWait, _ := m.takeKey(memoryGate, gateRule, now)
	if !gateOK && gateWait > wait {
		wait = gateWait
	}
	if !gateOK {
		return false, wait, false, nil
	}
	return true, 0, false, nil
}

func (m *Memory) takeKey(key string, rule Rule, now time.Time) (bool, time.Duration, bool) {
	st, ok := m.m[key]
	fresh := false
	if !ok {
		fresh = !isOverflowKey(key)
		st = bucketState{}
	}
	tokens := st.tokens
	ts := st.ts
	if !st.ready {
		tokens = float64(rule.Burst)
		ts = now
	}
	allowed, wait, next := takeToken(tokens, ts, now, rule)
	m.m[key] = bucketState{tokens: next, ts: now, ready: true}
	return allowed, wait, fresh
}

func (m *Memory) subjectCount() int {
	n := 0
	for key := range m.m {
		if !isOverflowKey(key) {
			n++
		}
	}
	return n
}

func overflowKey(key string) string {
	bucket := "unknown"
	rest := strings.TrimPrefix(key, "flood:")
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		bucket = rest[:i]
	} else if rest != "" {
		bucket = rest
	}
	return memoryOverflow + ":" + bucket
}

func isOverflowKey(key string) bool {
	return key == memoryOverflow || strings.HasPrefix(key, memoryOverflow+":")
}

// Drop removes a key inserted and then rejected in the same request.
func (m *Memory) Drop(key string) {
	if m == nil || key == "" || isOverflowKey(key) {
		return
	}
	m.mu.Lock()
	delete(m.m, key)
	m.mu.Unlock()
}

func (m *Memory) pruneLocked(now time.Time) {
	ttl := m.ttl
	if ttl <= 0 {
		ttl = memoryTTL
	}
	cut := now.Add(-ttl)
	for key, st := range m.m {
		if st.ready && !st.ts.After(cut) {
			delete(m.m, key)
		}
	}
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
