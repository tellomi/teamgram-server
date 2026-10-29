package verify

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memKV struct {
	mu sync.Mutex
	m  map[string]string
}

func (m *memKV) GetCtx(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[key], nil
}

func (m *memKV) SetexCtx(ctx context.Context, key, value string, seconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.m[key] = value
	return nil
}

func TestLimiterResendAndDailyCap(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 2, DailyIP: 5})
	lim.now = func() time.Time { return time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4")
	le, ok := err.(*LimitError)
	if !ok || le.Daily || le.Retry <= 0 {
		t.Fatalf("cooldown: %#v", err)
	}

	kv.m = map[string]string{}
	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	delete(kv.m, "tellomi:code:cd:sms:8613800138000")
	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	delete(kv.m, "tellomi:code:cd:sms:8613800138000")
	err = lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4")
	le, ok = err.(*LimitError)
	if !ok || !le.Daily {
		t.Fatalf("daily: %#v", err)
	}
}
