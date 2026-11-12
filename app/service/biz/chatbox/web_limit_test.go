package chatbox

import (
	"testing"
	"time"
)

func TestAllowSendFetchUserBudget(t *testing.T) {
	resetFetchLimits()
	const user int64 = 42
	for i := 0; i < webUserBurst; i++ {
		if !AllowSendFetch(user) {
			t.Fatalf("token %d was refused", i)
		}
	}
	if AllowSendFetch(user) {
		t.Fatal("user over budget still got a fetch")
	}
	// A different user is unaffected.
	if !AllowSendFetch(user + 1) {
		t.Fatal("other user was blocked")
	}
	// No error is returned; the attach path just skips the card.
}

func TestAllowSendFetchGlobalCap(t *testing.T) {
	resetFetchLimits()
	for i := 0; i < webGlobalSlots; i++ {
		webSlots <- struct{}{}
	}
	t.Cleanup(resetFetchLimits)
	const user int64 = 99
	if AllowSendFetch(user) {
		t.Fatal("global cap still allowed a new fetch")
	}
	// Refund: after a slot frees, the same user can start one.
	<-webSlots
	if !AllowSendFetch(user) {
		t.Fatal("user budget was burned by the global cap refusal")
	}
}

func TestAllowSendFetchZeroUser(t *testing.T) {
	resetFetchLimits()
	if !AllowSendFetch(0) {
		t.Fatal("missing user id was treated as a budget miss")
	}
}

func TestUserBudgetRefills(t *testing.T) {
	resetFetchLimits()
	const user int64 = 7
	now := time.Now()
	if !userAllow(user, now) {
		t.Fatal("first token")
	}
	// Drain.
	for i := 1; i < webUserBurst; i++ {
		if !userAllow(user, now) {
			t.Fatalf("drain %d", i)
		}
	}
	if userAllow(user, now) {
		t.Fatal("expected empty")
	}
	// One token every 3 seconds at 20/min.
	if !userAllow(user, now.Add(4*time.Second)) {
		t.Fatal("budget did not refill")
	}
}
