package uploadledger

import (
	"context"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type redisAdder interface {
	add(kind string, user, n, limit int64, window time.Duration, enforce bool) error
	reserve(user, n, limit int64, window time.Duration, enforce bool) (int64, error)
	cancel(id int64)
	release(user, n int64)
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

const addScript = `
local add = tonumber(ARGV[1])
local lim = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local enforce = ARGV[4]
local now = tonumber(redis.call('TIME')[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
local rows = redis.call('ZRANGE', KEYS[1], 0, -1)
local cur = 0
for _, member in ipairs(rows) do
  local amt = tonumber(string.match(member, ':(%d+)$')) or 0
  cur = cur + amt
end
if enforce == '1' and lim > 0 and cur + add > lim then
  return {0, window}
end
local id = redis.call('INCR', KEYS[2])
redis.call('ZADD', KEYS[1], now, id .. ':' .. add)
redis.call('EXPIRE', KEYS[1], window)
redis.call('EXPIRE', KEYS[2], window)
return {1, window}
`

const sumScript = `
local now = tonumber(redis.call('TIME')[1])
local window = tonumber(ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
local rows = redis.call('ZRANGE', KEYS[1], 0, -1)
local cur = 0
for _, member in ipairs(rows) do
  cur = cur + (tonumber(string.match(member, ':(%d+)$')) or 0)
end
return cur
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
redis.call('HSET', KEYS[3], tostring(id), add)
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
local n = redis.call('HGET', 'tellomi:up:holds:' .. user, ARGV[1])
if not n then return 0 end
redis.call('HDEL', 'tellomi:up:holds:' .. user, ARGV[1])
redis.call('DECRBY', 'tellomi:up:hold:' .. user, n)
redis.call('DEL', KEYS[1])
return 1
`

func (b *redisBackend) add(kind string, user, n, limit int64, window time.Duration, enforce bool) error {
	key := "tellomi:up:" + kind + ":" + strconv.FormatInt(user, 10)
	flag := "0"
	if enforce {
		flag = "1"
	}
	res, err := b.r.EvalCtx(context.Background(), addScript, []string{key, key + ":seq"},
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

func (b *redisBackend) reserve(user, n, limit int64, window time.Duration, enforce bool) (int64, error) {
	userS := strconv.FormatInt(user, 10)
	flag := "0"
	if enforce {
		flag = "1"
	}
	res, err := b.r.EvalCtx(context.Background(), reserveScript, []string{
		"tellomi:up:hold:" + userS,
		"tellomi:up:holdseq",
		"tellomi:up:holds:" + userS,
	}, n, limit, int(window/time.Second), flag)
	if err != nil {
		if enforce {
			return 0, &Limited{Wait: time.Second}
		}
		return 0, nil
	}
	ok, id := pair(res)
	if ok == 0 {
		return 0, &Limited{Wait: time.Duration(id) * time.Second}
	}
	_, _ = b.r.EvalCtx(context.Background(),
		`redis.call('SET', KEYS[1], ARGV[1]); redis.call('EXPIRE', KEYS[1], ARGV[2]); return 1`,
		[]string{"tellomi:up:holdid:" + strconv.FormatInt(id, 10)},
		userS, int(window/time.Second))
	return id, nil
}

func (b *redisBackend) cancel(id int64) {
	_, _ = b.r.EvalCtx(context.Background(), cancelScript,
		[]string{"tellomi:up:holdid:" + strconv.FormatInt(id, 10)},
		strconv.FormatInt(id, 10))
}

func (b *redisBackend) release(user, n int64) {
	key := "tellomi:up:hold:" + strconv.FormatInt(user, 10)
	_, _ = b.r.EvalCtx(context.Background(), `
local cur = tonumber(redis.call('GET', KEYS[1]) or '0')
local drop = tonumber(ARGV[1])
if drop > cur then drop = cur end
if drop > 0 then redis.call('DECRBY', KEYS[1], drop) end
return drop
`, []string{key}, n)
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
	res, err := b.r.EvalCtx(context.Background(), sumScript,
		[]string{"tellomi:up:" + kind + ":" + strconv.FormatInt(user, 10)}, window)
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
