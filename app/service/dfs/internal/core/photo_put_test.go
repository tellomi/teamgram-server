package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPhotoPutsRunConcurrently(t *testing.T) {
	const (
		n    = 4
		each = 80 * time.Millisecond
	)
	var current atomic.Int32
	var maxSeen atomic.Int32
	fns := make([]func(context.Context) error, n)
	for i := range fns {
		fns[i] = func(context.Context) error {
			got := current.Add(1)
			for {
				old := maxSeen.Load()
				if got <= old || maxSeen.CompareAndSwap(old, got) {
					break
				}
			}
			time.Sleep(each)
			current.Add(-1)
			return nil
		}
	}
	start := time.Now()
	if err := runBounded(context.Background(), photoPutConcurrency, fns); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if maxSeen.Load() < 2 {
		t.Fatalf("peak in flight %d, puts were serialized", maxSeen.Load())
	}
	// Four sequential sleeps would be ~320ms. Bounded parallelism finishes
	// near one sleep. The bound is loose so a loaded CI runner still passes.
	if elapsed > each*2+40*time.Millisecond {
		t.Fatalf("4 puts took %s, want about %s in parallel", elapsed, each)
	}
	t.Logf("4x%dms puts finished in %s (peak %d)", each.Milliseconds(), elapsed, maxSeen.Load())
}

func TestPhotoPutErrorCancelsTheRest(t *testing.T) {
	started := atomic.Int32{}
	fns := []func(context.Context) error{
		func(ctx context.Context) error {
			started.Add(1)
			<-ctx.Done()
			return ctx.Err()
		},
		func(context.Context) error {
			started.Add(1)
			return context.Canceled
		},
	}
	err := runBounded(context.Background(), 2, fns)
	if err == nil {
		t.Fatal("expected an error")
	}
}
