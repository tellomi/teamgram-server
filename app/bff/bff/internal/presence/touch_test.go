package presence

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestShouldTouchThrottles(t *testing.T) {
	if !ShouldTouch(0, 100, TouchInterval) {
		t.Fatal("first write")
	}
	if ShouldTouch(100, 100+TouchInterval-1, TouchInterval) {
		t.Fatal("inside the window")
	}
	if !ShouldTouch(100, 100+TouchInterval, TouchInterval) {
		t.Fatal("at the window")
	}
	if ShouldTouch(100, 0, TouchInterval) || ShouldTouch(100, 200, 0) {
		t.Fatal("missing clock or interval")
	}
}

func TestSkipUpdateStatus(t *testing.T) {
	if !SkipMethod("/mtproto.RPCUserChannelProfiles/account_updateStatus") {
		t.Fatal("updateStatus sets the row itself")
	}
	if SkipMethod("/mtproto.RPCMessages/messages_sendMessage") {
		t.Fatal("an ordinary RPC should refresh last seen")
	}
}

func TestToucherWritesOnceUntilForgotten(t *testing.T) {
	var got []int64
	var mu sync.Mutex
	tchr := NewToucher(TouchInterval, func(_ context.Context, userID, now int64) {
		mu.Lock()
		got = append(got, userID, now)
		mu.Unlock()
	})
	tchr.now = func() int64 { return 1_000 }
	tchr.Touch(context.Background(), 7)
	tchr.Touch(context.Background(), 0)
	tchr.Touch(context.Background(), 7)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if len(got) != 2 || got[0] != 7 || got[1] != 1_000 {
		mu.Unlock()
		t.Fatalf("writes %v", got)
	}
	mu.Unlock()

	tchr.Forget(7)
	tchr.Touch(context.Background(), 7)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 4 || got[2] != 7 || got[3] != 1_000 {
		t.Fatalf("forget should allow another write, got %v", got)
	}
}
