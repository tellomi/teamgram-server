package verify

import (
	"context"
	"strconv"
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

func (m *memKV) IncrCtx(ctx context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	n := 1
	if v := m.m[key]; v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			n = parsed + 1
		}
	}
	m.m[key] = strconv.Itoa(n)
	return int64(n), nil
}

func (m *memKV) ExpireCtx(context.Context, string, int) error { return nil }

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

func TestDailyCapUsesShanghaiDayAndIsAtomic(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 10, DailyIP: 0})
	lim.limits.ResendSeconds = 0
	// 2026-11-07 15:30 UTC is 23:30 in Asia/Shanghai, still the 7th.
	late := time.Date(2026, 11, 7, 15, 30, 0, 0, time.UTC)
	lim.now = func() time.Time { return late }
	ctx := context.Background()

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- lim.AllowSend(ctx, "sms", "8613800138000", "8.8.8.8")
		}()
	}
	wg.Wait()
	close(errCh)
	okN, floodN := 0, 0
	for err := range errCh {
		if err == nil {
			okN++
			continue
		}
		le, ok := err.(*LimitError)
		if !ok || !le.Daily || le.Retry < 60 || le.Retry > 3600 {
			t.Fatalf("concurrent: %#v", err)
		}
		floodN++
	}
	if okN != 10 || floodN != 10 {
		t.Fatalf("allowed %d blocked %d", okN, floodN)
	}
	if _, exists := kv.m["tellomi:code:day:sms:ip:8.8.8.8:20261107"]; exists {
		t.Fatal("ip cap is off by default and must not be counted")
	}

	// 16:30 UTC is 00:30 the next Shanghai day. The counter starts over.
	lim.now = func() time.Time { return time.Date(2026, 11, 7, 16, 30, 0, 0, time.UTC) }
	if err := lim.AllowSend(ctx, "sms", "8613800138000", ""); err != nil {
		t.Fatal(err)
	}
	if kv.m["tellomi:code:day:sms:phone:8613800138000:20261108"] != "1" {
		t.Fatalf("next day %#v", kv.m)
	}
}

func TestEmailAndSMSQuotasAreSeparate(t *testing.T) {
	kv := &memKV{}
	lim := NewLimiter(kv, Limits{ResendSeconds: 60, DailyPhone: 1, DailyIP: 5})
	lim.now = func() time.Time { return time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	if err := lim.AllowSend(ctx, "sms", "8613800138000", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if err := lim.AllowSend(ctx, "email", "user@tellomi.app", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
}
