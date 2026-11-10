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
	for {
		select {
		case <-webSlots:
		default:
			return
		}
	}
}
