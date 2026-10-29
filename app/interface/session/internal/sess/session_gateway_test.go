package sess

import (
	"testing"
	"time"
)

func TestStaleGatewayCloseKeepsReboundSession(t *testing.T) {
	s := &session{}
	s.setGatewayId("10.0.0.8:10443")
	if s.getGatewayId() == "" {
		t.Fatal("bind did not stick")
	}

	stale := time.Now().Add(-time.Second).UnixNano()
	if s.releaseGateway("10.0.0.8:10443", stale) {
		t.Fatal("close from before the reconnect cleared the gateway")
	}
	if got := s.getGatewayId(); got != "10.0.0.8:10443" {
		t.Fatalf("gateway = %q", got)
	}
	if s.releaseGateway("10.0.0.9:10443", time.Now().Add(time.Second).UnixNano()) {
		t.Fatal("a different gateway cleared the binding")
	}
	if s.releaseGateway("10.0.0.8:10443", 0) {
		t.Fatal("unstamped close immediately after bind was applied")
	}

	if !s.releaseGateway("10.0.0.8:10443", time.Now().Add(time.Second).UnixNano()) {
		t.Fatal("close after the bind was ignored")
	}
	if s.getGatewayId() != "" {
		t.Fatal("gateway still set after a real close")
	}
}

func TestSplitCloseStamp(t *testing.T) {
	ip, at := SplitCloseStamp("1.2.3.4|1700000000123")
	if ip != "1.2.3.4" || at != 1700000000123 {
		t.Fatalf("parsed %q %d", ip, at)
	}
	ip, at = SplitCloseStamp("1.2.3.4")
	if ip != "1.2.3.4" || at != 0 {
		t.Fatalf("unstamped %q %d", ip, at)
	}
}

func TestEmptyGatewayIDIsNotBound(t *testing.T) {
	s := &session{}
	s.setGatewayId("")
	if s.getGatewayId() != "" {
		t.Fatal("empty gateway id was stored")
	}
}
