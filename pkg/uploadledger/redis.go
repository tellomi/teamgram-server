package uploadledger

import (
	"context"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type redisAdder interface {
	add(kind string, user, n, limit int64, window time.Duration, enforce bool) error
	probe(kind string, user, n, limit int64, window time.Duration, enforce bool) error
	refund(kind string, user, n int64, window time.Duration)
	reserve(user, fileID, n, limit int64, window time.Duration, enforce bool) (int64, error)
	cancel(id int64)
	releaseFile(user, fileID int64)
	book(user int64, window time.Duration) Book
}

type redisBackend struct {
	r *redis.Redis
}

func openRedis(addr string, lim Limits, mode string) (*Ledger, error) {
	rds, err := redis.NewRedis(redis.RedisConf{Host: addr, Type: redis.NodeType})
	if err != nil {
		return nil, err
	}
	led := newLedger(lim, mode)
	led.redis = &redisBackend{r: rds}
	return led, nil
}

// Meter bytes live in at most 60 time buckets. The hash field is the bucket
// start and the zset score is the latest write into that bucket, so a merged
// bucket keeps the newer timestamp. A full scan is a scan of those buckets.
const addScript = `
local add = tonumber(ARGV[1])
local lim = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local enforce = ARGV[4]
local now = tonumber(redis.call('TIME')[1])
local width = math.floor(window / 60)
if width < 1 then width = 1 end
local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now - window)
for _, b in ipairs(expired) do
  redis.call('HDEL', KEYS[1], b)
end
if #expired > 0 then
  redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - window)
end
local fields = redis.call('HGETALL', KEYS[1])
local cur = 0
for i = 2, #fields, 2 do
  cur = cur + (tonumber(fields[i]) or 0)
end
if enforce == '1' and lim > 0 and cur + add > lim then
  return {0, window}
end
local bucket = tostring(now - (now % width))
redis.call('HINCRBY', KEYS[1], bucket, add)
redis.call('ZADD', KEYS[2], now, bucket)
redis.call('EXPIRE', KEYS[1], window + width)
redis.call('EXPIRE', KEYS[2], window + width)
return {1, window}
`

const probeScript = `
local add = tonumber(ARGV[1])
local lim = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local enforce = ARGV[4]
if enforce ~= '1' or lim <= 0 then
  return {1, window}
end
local now = tonumber(redis.call('TIME')[1])
local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now - window)
for _, b in ipairs(expired) do
  redis.call('HDEL', KEYS[1], b)
end
if #expired > 0 then
  redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - window)
end
local fields = redis.call('HGETALL', KEYS[1])
local cur = 0
for i = 2, #fields, 2 do
  cur = cur + (tonumber(fields[i]) or 0)
end
if cur + add > lim then
  return {0, window}
end
return {1, window}
`

const sumScript = `
local window = tonumber(ARGV[1])
local now = tonumber(redis.call('TIME')[1])
local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now - window)
for _, b in ipairs(expired) do
  redis.call('HDEL', KEYS[1], b)
end
if #expired > 0 then
  redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - window)
end
local fields = redis.call('HGETALL', KEYS[1])
local cur = 0
for i = 2, #fields, 2 do
  cur = cur + (tonumber(fields[i]) or 0)
end
return cur
`

const refundScript = `
local left = tonumber(ARGV[1])
local members = redis.call('ZREVRANGE', KEYS[2], 0, -1)
for _, b in ipairs(members) do
  if left <= 0 then break end
  local cur = tonumber(redis.call('HGET', KEYS[1], b) or '0')
  if cur <= left then
    left = left - cur
    redis.call('HDEL', KEYS[1], b)
    redis.call('ZREM', KEYS[2], b)
  else
    redis.call('HINCRBY', KEYS[1], b, -left)
    left = 0
  end
end
return left
`

const reserveScript = `
local total = tonumber(redis.call('GET', KEYS[1]) or '0')
local add = tonumber(ARGV[1])
local lim = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local enforce = ARGV[4]
if enforce == '1' and lim > 0 and total + add > lim then
  local left = redis.call('TTL', KEYS[1])
  if left < 1 then left = ttl end
  return {0, left}
end
local id = redis.call('INCR', KEYS[2])
local fileId = ARGV[5]
redis.call('HSET', KEYS[3], tostring(id), add .. ':' .. fileId)
redis.call('SET', 'tellomi:up:holdid:' .. id, ARGV[6])
redis.call('EXPIRE', 'tellomi:up:holdid:' .. id, ttl)
redis.call('INCRBY', KEYS[1], add)
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ttl)
  redis.call('EXPIRE', KEYS[3], ttl)
end
return {1, id}
`

const cancelScript = `
local user = redis.call('GET', KEYS[1])
if not user then return 0 end
local raw = redis.call('HGET', 'tellomi:up:holds:' .. user, ARGV[1])
if not raw then return 0 end
local n = tonumber(string.match(raw, '^(%d+)')) or 0
redis.call('HDEL', 'tellomi:up:holds:' .. user, ARGV[1])
if n > 0 then redis.call('DECRBY', 'tellomi:up:hold:' .. user, n) end
redis.call('DEL', KEYS[1])
return n
`

func (b *redisBackend) add(kind string, user, n, limit int64, window time.Duration, enforce bool) error {
	key := "tellomi:up:" + kind + ":" + strconv.FormatInt(user, 10)
	flag := "0"
	if enforce {
		flag = "1"
	}
	res, err := b.r.EvalCtx(context.Background(), addScript, []string{key, key + ":t"},
		n, limit, int(window/time.Second), flag)
	if err != nil {
		if enforce {
			return &Limited{Wait: time.Second}
		}
		return nil
	}
	ok, left := pair(res)
	if ok == 0 {
		wait := time.Duration(left) * time.Second
		if wait < time.Second {
			wait = window
		}
		return &Limited{Wait: wait}
	}
	return nil
}

func (b *redisBackend) probe(kind string, user, n, limit int64, window time.Duration, enforce bool) error {
	key := "tellomi:up:" + kind + ":" + strconv.FormatInt(user, 10)
	flag := "0"
	if enforce {
		flag = "1"
	}
	res, err := b.r.EvalCtx(context.Background(), probeScript, []string{key, key + ":t"},
		n, limit, int(window/time.Second), flag)
	if err != nil {
		if enforce {
			return &Limited{Wait: time.Second}
		}
		return nil
	}
	ok, left := pair(res)
	if ok == 0 {
		wait := time.Duration(left) * time.Second
		if wait < time.Second {
			wait = window
		}
		return &Limited{Wait: wait}
	}
	return nil
}

func (b *redisBackend) refund(kind string, user, n int64, window time.Duration) {
	key := "tellomi:up:" + kind + ":" + strconv.FormatInt(user, 10)
	_, _ = b.r.EvalCtx(context.Background(), refundScript, []string{key, key + ":t"}, n, int(window/time.Second))
}

func (b *redisBackend) reserve(user, fileID, n, limit int64, window time.Duration, enforce bool) (int64, error) {
	userS := strconv.FormatInt(user, 10)
	flag := "0"
	if enforce {
		flag = "1"
	}
	res, err := b.r.EvalCtx(context.Background(), reserveScript, []string{
		"tellomi:up:hold:" + userS,
		"tellomi:up:holdseq",
		"tellomi:up:holds:" + userS,
	}, n, limit, int(window/time.Second), flag, fileID, userS)
	if err != nil || res == nil {
		if !enforce {
			return 0, nil
		}
		return 0, &Limited{Wait: time.Second}
	}
	ok, id := pair(res)
	if ok == 0 {
		wait := time.Duration(id) * time.Second
		if wait < time.Second {
			wait = window
		}
		return 0, &Limited{Wait: wait}
	}
	if id == 0 {
		if !enforce {
			return 0, nil
		}
		return 0, &Limited{Wait: time.Second}
	}
	return id, nil
}

func (b *redisBackend) cancel(id int64) {
	_, _ = b.r.EvalCtx(context.Background(), cancelScript,
		[]string{"tellomi:up:holdid:" + strconv.FormatInt(id, 10)},
		strconv.FormatInt(id, 10))
}

func (b *redisBackend) releaseFile(user, fileID int64) {
	userS := strconv.FormatInt(user, 10)
	_, _ = b.r.EvalCtx(context.Background(), `
local want = ARGV[1]
local fields = redis.call('HGETALL', KEYS[1])
local dropped = 0
for i = 1, #fields, 2 do
  local id = fields[i]
  local n, file = string.match(fields[i + 1], '^(%d+):(%-?%d+)$')
  if file == want then
    redis.call('HDEL', KEYS[1], id)
    redis.call('DEL', 'tellomi:up:holdid:' .. id)
    dropped = dropped + (tonumber(n) or 0)
  end
end
if dropped > 0 then redis.call('DECRBY', KEYS[2], dropped) end
return dropped
`, []string{"tellomi:up:holds:" + userS, "tellomi:up:hold:" + userS}, strconv.FormatInt(fileID, 10))
}

func (b *redisBackend) book(user int64, window time.Duration) Book {
	sec := int(window / time.Second)
	return Book{
		Network: b.sum("net", user, sec),
		Entity:  b.sum("entity", user, sec),
		Refs:    b.sum("refs", user, sec),
		Hold:    b.num("hold", user),
	}
}

func (b *redisBackend) sum(kind string, user int64, window int) int64 {
	key := "tellomi:up:" + kind + ":" + strconv.FormatInt(user, 10)
	res, err := b.r.EvalCtx(context.Background(), sumScript,
		[]string{key, key + ":t"}, window)
	if err != nil {
		return 0
	}
	return asInt(res)
}

func (b *redisBackend) num(kind string, user int64) int64 {
	v, err := b.r.GetCtx(context.Background(), "tellomi:up:"+kind+":"+strconv.FormatInt(user, 10))
	if err != nil || v == "" {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func pair(res any) (int64, int64) {
	switch v := res.(type) {
	case []any:
		if len(v) < 2 {
			return 0, 0
		}
		return asInt(v[0]), asInt(v[1])
	case []int64:
		if len(v) < 2 {
			return 0, 0
		}
		return v[0], v[1]
	default:
		return 0, 0
	}
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
