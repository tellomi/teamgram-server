package privacymember

import (
	"context"
	"testing"
	"time"
)

func TestQueryTimeoutFires(t *testing.T) {
	ctx, cancel := withQueryTimeout(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("membership query has no deadline")
	}
	if wait := time.Until(deadline); wait <= 0 || wait > queryTimeout+50*time.Millisecond {
		t.Fatalf("deadline in %s, want about %s", wait, queryTimeout)
	}
}
