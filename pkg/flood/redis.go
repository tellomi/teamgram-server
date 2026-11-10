package flood

import (
	"context"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// tokenScript is the same bucket as takeToken. ARGV is now_ms, burst,
// tokens per millisecond, and a ttl in milliseconds.
const tokenScript = `
local now = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local rate = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local data = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil or ts == nil then
  tokens = burst
  ts = now
end
if now < ts then
  now = ts
end
local elapsed = now - ts
tokens = math.min(burst, tokens + elapsed * rate)
local allowed = 0
local wait = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  if rate > 0 then
    wait = math.ceil((1 - tokens) / rate)
  else
    wait = 3600000
  end
end
redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', tostring(now))
redis.call('PEXPIRE', KEYS[1], ttl)
return {allowed, wait}
`

// RedisStore keeps the buckets in the shared Redis.
type RedisStore struct {
	r *redis.Redis
}

func NewRedisStore(r *redis.Redis) *RedisStore {
	return &RedisStore{r: r}
}

func (s *RedisStore) Take(ctx context.Context, key string, rule Rule, now time.Time) (bool, time.Duration, error) {
	if s == nil || s.r == nil {
		return true, 0, nil
	}
	ttl := int64(120000)
	if rule.Rate > 0 {
		refill := int64(float64(rule.Burst)/rule.Rate*1000) + 60000
		if refill > ttl {
			ttl = refill
		}
	}
	if ttl > 86400000 {
		ttl = 86400000
	}
	ratePerMS := rule.Rate / 1000
	reply, err := s.r.EvalCtx(ctx, tokenScript, []string{key},
		now.UnixMilli(), rule.Burst, ratePerMS, ttl)
	if err != nil {
		return false, 0, err
	}
	allowed, wait, ok := parseEval(reply)
	if !ok {
		return true, 0, nil
	}
	return allowed, wait, nil
}

func parseEval(reply any) (bool, time.Duration, bool) {
	vals, ok := reply.([]any)
	if !ok || len(vals) < 2 {
		return false, 0, false
	}
	allowed := asInt(vals[0]) == 1
	wait := time.Duration(asInt(vals[1])) * time.Millisecond
	return allowed, wait, true
}

func asInt(v any) int64 {
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
