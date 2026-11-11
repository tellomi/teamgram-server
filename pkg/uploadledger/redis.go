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
	book(user int64) Book
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
local cur = tonumber(redis.call('GET', KEYS[1]) or '0')
local add = tonumber(ARGV[1])
local lim = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local enforce = ARGV[4]
if enforce == '1' and lim > 0 and cur + add > lim then
  local left = redis.call('TTL', KEYS[1])
  if left < 1 then left = ttl end
  return {0, left}
end
redis.call('INCRBY', KEYS[1], add)
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ttl)
end
local left = redis.call('TTL', KEYS[1])
if left < 0 then left = ttl end
return {1, left}
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
	res, err := b.r.EvalCtx(context.Background(), addScript, []string{key},
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

func (b *redisBackend) book(user int64) Book {
	return Book{
		Network: b.num("net", user),
		Entity:  b.num("entity", user),
		Refs:    b.num("refs", user),
		Hold:    b.num("hold", user),
	}
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
