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

// Package seqfloor keeps Redis id counters from restarting below ids that
// already exist in MySQL. INCR on a missing key returns 1, which is what
// put new private-chat messages above older ones after the Redis flush.
package seqfloor

import (
	"strconv"
)

// Redis key prefixes. These must stay aligned with idgen/client/vars.go.
const (
	MessageBoxPrefix        = "message_box_ngen_"
	ChannelMessageBoxPrefix = "channel_message_box_ngen_"
	SeqPrefix               = "seq_updates_ngen_"
	PtsPrefix               = "pts_updates_ngen_"
	QtsPrefix               = "qts_updates_ngen_"
	ChannelPtsPrefix        = "channel_pts_updates_ngen_"
	StoryPrefix             = "story_ngen_"
	ChannelStoryPrefix      = "channel_story_ngen_"
)

// allocLua raises the counter to floor when it is missing or lower, then
// increments. The whole update is one Redis script so two allocators cannot
// both observe a hole and hand out the same id.
//
// ARGV[1] is the database floor (last issued id, 0 if this key has no rows).
// ARGV[2] is the increment. The returned value is the new counter, which is
// the id (or the end of an n-id range) the caller stores.
const allocLua = `
local cur = redis.call('GET', KEYS[1])
local floor = tonumber(ARGV[1])
local n = tonumber(ARGV[2])
if floor > 0 and (not cur or tonumber(cur) < floor) then
  redis.call('SET', KEYS[1], tostring(floor))
end
return redis.call('INCRBY', KEYS[1], n)
`

// raiseLua sets the key to floor when it is missing or lower. It never
// decreases a counter that is already ahead (operators may have bumped it).
const raiseLua = `
local cur = redis.call('GET', KEYS[1])
local floor = tonumber(ARGV[1])
if floor > 0 and (not cur or tonumber(cur) < floor) then
  redis.call('SET', KEYS[1], tostring(floor))
  return 1
end
return 0
`

// reserveLua claims n ids above max(redis, floor) and stores the last id.
// It never lowers a counter. ARGV[1] is the database max, ARGV[2] is how
// many ids to claim. The return value is the last id now stored.
const reserveLua = `
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

// ReserveRange is the pure form of reserveLua.
// current is the Redis string (empty means missing). floor is the highest id
// already stored in MySQL. n is how many new ids to claim.
// first is the first id in the claimed range. last is the value Redis must hold
// afterwards. When n is 0, last is the current floor and first is last+1.
func ReserveRange(current string, floor, n int64) (first, last int64) {
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

// NextCounter is the pure form of allocLua.
// current is the Redis string; empty means the key is missing.
// floor is the last id already stored in MySQL.
// n is the increment (at least 1).
// The returned issued value is what INCRBY would return after the floor is applied.
func NextCounter(current string, floor, n int64) (issued int64) {
	var cur int64
	missing := current == ""
	if !missing {
		parsed, err := strconv.ParseInt(current, 10, 64)
		if err != nil {
			missing = true
		} else {
			cur = parsed
		}
	}
	if floor > 0 && (missing || cur < floor) {
		cur = floor
	}
	return cur + n
}

// ShouldRaise reports whether a startup heal must write floor into Redis.
// A counter that is already at or above the database max is left alone.
func ShouldRaise(current string, floor int64) bool {
	if floor <= 0 {
		return false
	}
	if current == "" {
		return true
	}
	cur, err := strconv.ParseInt(current, 10, 64)
	if err != nil {
		return true
	}
	return cur < floor
}
