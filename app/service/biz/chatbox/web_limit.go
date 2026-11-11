package chatbox

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/flood"
)

const (
	webDomainBurst = 5
	webDomainRate  = 1.0 // tokens per second
	webGlobalSlots = 8
	// Per-user budget for starting a new external fetch (sendMessage
	// pending cards). 20/minute with a burst of 20.
	webUserBurst = 20
	webUserRate  = 20.0 / 60.0
)

// acquireFetch spends one per-domain token and one global fetch slot.
// Over the limit, ok is false and the caller returns webPageEmpty without
// opening a connection. release is nil when ok is false.
func acquireFetch(host string) (release func(), ok bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || !domainAllow(host, time.Now()) {
		return nil, false
	}
	if release, decided, ok := acquireRedisSlot(); decided {
		if !ok {
			return nil, false
		}
		return release, true
	}
	select {
	case webSlots <- struct{}{}:
		return func() { <-webSlots }, true
	default:
		return nil, false
	}
}

// slotScript increments a shared counter and refuses past the cap.
// A crash leaves the key to expire instead of pinning the cap at full.
const slotScript = `
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], 30000)
end
if n > tonumber(ARGV[1]) then
  redis.call('DECR', KEYS[1])
  return 0
end
return 1
`

// acquireRedisSlot reports decided when Redis answered. ok is the slot.
// decided false means Redis is absent or the call failed, and the caller
// uses the in-process cap instead.
func acquireRedisSlot() (release func(), decided, ok bool) {
	r := linkRedis()
	if r == nil {
		return nil, false, false
	}
	reply, err := r.EvalCtx(context.Background(), slotScript, []string{"webpage:slots"}, webGlobalSlots)
	if err != nil {
		return nil, false, false
	}
	if asSlot(reply) != 1 {
		return nil, true, false
	}
	return func() { _, _ = r.Decr("webpage:slots") }, true, true
}

func asSlot(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	case []byte:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	default:
		return 0
	}
}

var webSlots = make(chan struct{}, webGlobalSlots)

type domainBucket struct {
	tokens float64
	ts     time.Time
	init   bool
}

var (
	domainMu sync.Mutex
	domains  = map[string]*domainBucket{}
)

func domainAllow(host string, now time.Time) bool {
	if r := linkRedis(); r != nil {
		allowed, _, err := flood.NewRedisStore(r).Take(context.Background(), "webpage:domain:"+host, flood.Rule{
			Burst: webDomainBurst,
			Rate:  webDomainRate,
		}, now)
		if err == nil {
			return allowed
		}
	}
	domainMu.Lock()
	defer domainMu.Unlock()
	b := domains[host]
	if b == nil {
		b = &domainBucket{}
		domains[host] = b
	}
	if !b.init {
		b.tokens = webDomainBurst
		b.ts = now
		b.init = true
	}
	elapsed := now.Sub(b.ts).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * webDomainRate
		if b.tokens > webDomainBurst {
			b.tokens = webDomainBurst
		}
		b.ts = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func resetFetchLimits() {
	domainMu.Lock()
	domains = map[string]*domainBucket{}
	domainMu.Unlock()
	userMu.Lock()
	users = map[int64]*domainBucket{}
	userMu.Unlock()
	for {
		select {
		case <-webSlots:
		default:
			return
		}
	}
}

var (
	userMu sync.Mutex
	users  = map[int64]*domainBucket{}
)

// AllowSendFetch is the attach-time budget for a new external card.
// It spends one per-user token and refuses when the process already has
// webGlobalSlots fetches in flight. Over the limit, ok is false and the
// caller attaches no card and returns no error.
func AllowSendFetch(userID int64) bool {
	if !userAllow(userID, time.Now()) {
		return false
	}
	if !globalSlotAvailable() {
		// Refund the user token so a full global cap does not burn budget.
		refundUser(userID, time.Now())
		return false
	}
	return true
}

func globalSlotAvailable() bool {
	if r := linkRedis(); r != nil {
		n, err := r.Get("webpage:slots")
		if err == nil {
			i, convErr := strconv.ParseInt(n, 10, 64)
			if convErr == nil {
				return i < int64(webGlobalSlots)
			}
		}
		// Redis present but unreadable: fall through to the in-process peek.
	}
	return len(webSlots) < webGlobalSlots
}

func userAllow(userID int64, now time.Time) bool {
	if userID == 0 {
		return true
	}
	if r := linkRedis(); r != nil {
		allowed, _, err := flood.NewRedisStore(r).Take(context.Background(), "webpage:user:"+strconv.FormatInt(userID, 10), flood.Rule{
			Burst: webUserBurst,
			Rate:  webUserRate,
		}, now)
		if err == nil {
			return allowed
		}
	}
	userMu.Lock()
	defer userMu.Unlock()
	return takeBucket(users, userID, webUserBurst, webUserRate, now)
}

func refundUser(userID int64, now time.Time) {
	if userID == 0 {
		return
	}
	userMu.Lock()
	defer userMu.Unlock()
	b := users[userID]
	if b == nil || !b.init {
		return
	}
	b.tokens++
	if b.tokens > webUserBurst {
		b.tokens = webUserBurst
	}
	b.ts = now
}

func takeBucket(m map[int64]*domainBucket, id int64, burst int, rate float64, now time.Time) bool {
	b := m[id]
	if b == nil {
		b = &domainBucket{}
		m[id] = b
	}
	if !b.init {
		b.tokens = float64(burst)
		b.ts = now
		b.init = true
	}
	elapsed := now.Sub(b.ts).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rate
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
		b.ts = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// ResetFetchLimitsForTest empties domain, user, and global fetch caps.
func ResetFetchLimitsForTest() { resetFetchLimits() }
