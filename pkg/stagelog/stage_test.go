package stagelog

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
)

func TestMarksRoundTrip(t *testing.T) {
	gw := time.Unix(0, 100)
	enq := time.Unix(0, 200)
	loop := time.Unix(0, 350)
	ctx := WithMarks(context.Background(), gw, time.Time{}, time.Time{})
	ctx = WithMarks(ctx, time.Time{}, enq, loop)
	gotGW, gotEnq, gotLoop := Marks(ctx)
	if !gotGW.Equal(gw) || !gotEnq.Equal(enq) || !gotLoop.Equal(loop) {
		t.Fatalf("marks = %v %v %v", gotGW, gotEnq, gotLoop)
	}
}

func TestStampGatewayReadsHeader(t *testing.T) {
	enabledOnce.Do(func() {})
	prev := enabled
	enabled = true
	t.Cleanup(func() { enabled = prev })

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(GatewayHeader, "not-a-number"))
	gw, _, _ := Marks(StampGateway(ctx))
	if !gw.IsZero() {
		t.Fatal("bad header was stored")
	}
	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs(GatewayHeader, "1700000000000000000"))
	gw, _, _ = Marks(StampGateway(ctx))
	if gw.UnixNano() != 1700000000000000000 {
		t.Fatalf("gateway clock = %d", gw.UnixNano())
	}
}

func TestBetween(t *testing.T) {
	a := time.Unix(10, 0)
	b := a.Add(1500 * time.Millisecond)
	if between(a, b) != 1500 {
		t.Fatalf("between = %d", between(a, b))
	}
	if between(time.Time{}, b) != -1 || between(b, a) != -1 {
		t.Fatal("missing or reversed clocks should be -1")
	}
}

func TestInfofStaysQuietWhenOff(t *testing.T) {
	enabledOnce.Do(func() {})
	prev := enabled
	enabled = false
	t.Cleanup(func() { enabled = prev })
	Infof("should not matter %d", 1)
	SessionDone("m", 1, 2, time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), time.Now(), 0)
}
