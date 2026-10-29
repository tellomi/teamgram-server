// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package seqrepair

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// Redis key prefixes. Keep these aligned with idgen/client/vars.go.
const (
	messageBoxPrefix = "message_box_ngen_"
	ptsPrefix        = "pts_updates_ngen_"
)

// Sequencer reads and reserves idgen counters. Reserve must be atomic so a
// send that arrives while the repair is writing cannot reuse an id.
type Sequencer interface {
	Get(ctx context.Context, key string) (string, error)
	Reserve(ctx context.Context, key string, floor, n int64) (first, last int64, err error)
}

// RedisSeq is the idgen Redis.
type RedisSeq struct {
	r *redis.Redis
}

func DialRedis(addr string) *RedisSeq {
	return &RedisSeq{r: redis.New(addr)}
}

func (s *RedisSeq) Get(ctx context.Context, key string) (string, error) {
	return s.r.GetCtx(ctx, key)
}

func (s *RedisSeq) Reserve(ctx context.Context, key string, floor, n int64) (int64, int64, error) {
	v, err := s.r.EvalCtx(ctx, reserveScript, []string{key}, floor, n)
	if err != nil {
		return 0, 0, err
	}
	last, err := asInt64(v)
	if err != nil {
		return 0, 0, err
	}
	if n <= 0 {
		return last + 1, last, nil
	}
	return last - n + 1, last, nil
}

// reserveScript matches seqfloor.ReserveRange. Inlined because the seqfloor
// script is unexported; the test TestReserveRangeStaysAboveTheLiveCounter is
// the spec both sides follow.
const reserveScript = `
local cur = redis.call('GET', KEYS[1])
local floor = tonumber(ARGV[1])
local n = tonumber(ARGV[2])
local base = 0
if cur and cur ~= false then
  base = tonumber(cur)
end
if floor > base then
  base = floor
end
if n <= 0 then
  return base
end
local last = base + n
redis.call('SET', KEYS[1], tostring(last))
return last
`

// MemSeq is an in-memory sequencer for tests.
type MemSeq struct {
	mu sync.Mutex
	v  map[string]string
}

func NewMemSeq(values map[string]string) *MemSeq {
	cp := map[string]string{}
	for k, val := range values {
		cp[k] = val
	}
	return &MemSeq{v: cp}
}

func (m *MemSeq) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.v[key], nil
}

func (m *MemSeq) Reserve(_ context.Context, key string, floor, n int64) (int64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	first, last := reserveRange(m.v[key], floor, n)
	if n > 0 {
		m.v[key] = strconv.FormatInt(last, 10)
	}
	return first, last, nil
}

func (m *MemSeq) Value(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.v[key]
}

func boxKey(user int64) string {
	return messageBoxPrefix + strconv.FormatInt(user, 10)
}

func ptsKey(user int64) string {
	return ptsPrefix + strconv.FormatInt(user, 10)
}

// reserveRange matches the Redis script below and seqfloor.ReserveRange.
func reserveRange(current string, floor, n int64) (first, last int64) {
	var base int64
	if current != "" {
		parsed, err := strconv.ParseInt(current, 10, 64)
		if err == nil && parsed > base {
			base = parsed
		}
	}
	if floor > base {
		base = floor
	}
	if n <= 0 {
		return base + 1, base
	}
	return base + 1, base + n
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int:
		return int64(n), nil
	case float64:
		return int64(n), nil
	case string:
		return strconv.ParseInt(n, 10, 64)
	case []byte:
		return strconv.ParseInt(string(n), 10, 64)
	default:
		return 0, fmt.Errorf("redis eval result %T", v)
	}
}
