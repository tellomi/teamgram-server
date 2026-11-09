package postsearch

import (
	"context"
	"testing"
	"time"
)

func TestQuotaIsFreeAndResetsInShanghai(t *testing.T) {
	t.Setenv(envDaily, "3")
	day := time.Date(2026, 11, 9, 23, 30, 0, 0, shanghai)
	b := &Book{kv: newMem(), now: func() time.Time { return day }}
	ctx := context.Background()

	got := b.Peek(ctx, 42)
	if !got.GetQueryIsFree() || got.GetStarsAmount() != 0 || got.GetTotalDaily() != 3 || got.GetRemains() != 3 || got.GetWaitTill() != nil {
		t.Fatalf("peek %+v", got)
	}

	for i := int32(3); i >= 1; i-- {
		got = b.Spend(ctx, 42)
		if got.GetRemains() != i-1 || got.GetStarsAmount() != 0 || !got.GetQueryIsFree() {
			t.Fatalf("spend remains=%d flood=%+v", i-1, got)
		}
		if i-1 > 0 && got.GetWaitTill() != nil {
			t.Fatalf("wait while remains %d: %+v", i-1, got.GetWaitTill())
		}
	}
	if got.GetWaitTill().GetValue() != int32(time.Date(2026, 11, 10, 0, 0, 0, 0, shanghai).Unix()) {
		t.Fatalf("wait_till %v", got.GetWaitTill())
	}
	again := b.Spend(ctx, 42)
	if again.GetRemains() != 0 || again.GetStarsAmount() != 0 {
		t.Fatalf("past cap %+v", again)
	}

	b.now = func() time.Time { return day.Add(2 * time.Hour) }
	reset := b.Peek(ctx, 42)
	if reset.GetRemains() != 3 || reset.GetWaitTill() != nil {
		t.Fatalf("next shanghai day %+v", reset)
	}
}

func TestDailyCapDefault(t *testing.T) {
	t.Setenv(envDaily, "")
	if dailyCap() != 100 {
		t.Fatalf("default %d", dailyCap())
	}
	t.Setenv(envDaily, "nope")
	if dailyCap() != 100 {
		t.Fatalf("bad env %d", dailyCap())
	}
}
