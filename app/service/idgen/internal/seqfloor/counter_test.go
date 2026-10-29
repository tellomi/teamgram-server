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

package seqfloor

import "testing"

func TestNextCounterNeverGoesBackwards(t *testing.T) {
	// DB max for erel's box is 51, so the next id must be 52 even when Redis
	// was flushed (missing) or has already handed out 1..7.
	tests := []struct {
		name    string
		current string
		floor   int64
		n       int64
		want    int64
	}{
		{name: "flushed key starts at db max plus one", current: "", floor: 51, n: 1, want: 52},
		{name: "restarted counter is raised before increment", current: "5", floor: 51, n: 1, want: 52},
		{name: "counter equal to db max increments", current: "51", floor: 51, n: 1, want: 52},
		{name: "ops bump above db max is not lowered", current: "1051", floor: 51, n: 1, want: 1052},
		{name: "mke box floor", current: "", floor: 40, n: 1, want: 41},
		{name: "pts range keeps the end of the range above the floor", current: "2", floor: 80, n: 3, want: 83},
		{name: "no rows yet starts at one", current: "", floor: 0, n: 1, want: 1},
		{name: "garbage redis value is treated as missing", current: "nope", floor: 51, n: 1, want: 52},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextCounter(tt.current, tt.floor, tt.n)
			if got != tt.want {
				t.Fatalf("NextCounter(%q, %d, %d) = %d, want %d", tt.current, tt.floor, tt.n, got, tt.want)
			}
			if tt.floor > 0 && got <= tt.floor {
				t.Fatalf("issued %d is not above db floor %d", got, tt.floor)
			}
		})
	}
}

func TestReserveRangeStaysAboveTheLiveCounter(t *testing.T) {
	// Erel already has box 1052 in MySQL and Redis was bumped again, so the
	// counter is 2052. Three rebuilt messages must start at 2053, and the
	// counter must land on the last of them.
	first, last := ReserveRange("2052", 1052, 3)
	if first != 2053 || last != 2055 {
		t.Fatalf("ReserveRange(2052, 1052, 3) = %d..%d, want 2053..2055", first, last)
	}
	// A missing key still cannot start at 1 when the table already has ids.
	first, last = ReserveRange("", 51, 2)
	if first != 52 || last != 53 {
		t.Fatalf("ReserveRange(missing, 51, 2) = %d..%d, want 52..53", first, last)
	}
	// Claiming nothing does not move the counter.
	first, last = ReserveRange("2153", 1153, 0)
	if last != 2153 || first != 2154 {
		t.Fatalf("ReserveRange no-op = first %d last %d", first, last)
	}
}

func TestShouldRaiseDoesNotLowerABump(t *testing.T) {
	if ShouldRaise("1051", 51) {
		t.Fatal("counter already at db max+1000 must not be rewritten")
	}
	if ShouldRaise("51", 51) {
		t.Fatal("counter equal to the floor is already safe")
	}
	if !ShouldRaise("", 51) {
		t.Fatal("missing counter must be initialized from the database")
	}
	if !ShouldRaise("7", 51) {
		t.Fatal("counter below the database max must be raised")
	}
	if ShouldRaise("", 0) {
		t.Fatal("a user with no rows does not need a redis key")
	}
}
