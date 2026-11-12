package password

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/verify"
)

// The cloud-password email sender is the closure ConfigureFromEnv installs.
// An empty Resend key fails inside the sender without an HTTP call. The next
// send, still on the same Shanghai day, is PHONE_NUMBER_FLOOD from the quota
// and never reaches that sender.
func TestPasswordEmailEleventhDoesNotReachSender(t *testing.T) {
	t.Setenv("TELLOMI_EMAIL_PROVIDER", "resend")
	t.Setenv("TELLOMI_RESEND_API_KEY", "")
	t.Setenv("TELLOMI_EMAIL_FROM", "noreply@tellomi.app")

	kv := &mailKV{}
	lim := verify.NewLimiter(kv, verify.Limits{ResendSeconds: 60, DailyPhone: 1})
	verify.UseSharedLimiters(nil, lim)
	prev := Swap(New(Options{}))
	t.Cleanup(func() {
		verify.UseSharedLimiters(nil, nil)
		Swap(prev)
	})
	ConfigureFromEnv("")

	ctx := context.Background()
	email := "user@example.com"
	_, err := Default().sendCode(ctx, email)
	if err == nil || !strings.Contains(err.Error(), "SMS_CODE_CREATE_FAILED") {
		t.Fatalf("first send: %v", err)
	}
	kv.del("tellomi:code:cd:email:" + email)
	_, err = Default().sendCode(ctx, email)
	if err == nil || !strings.Contains(err.Error(), "PHONE_NUMBER_FLOOD") {
		t.Fatalf("second send: %v", err)
	}
}

type mailKV struct {
	mu sync.Mutex
	m  map[string]string
}

func (m *mailKV) del(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}

func (m *mailKV) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		return "", nil
	}
	return m.m[key], nil
}

func (m *mailKV) SetexCtx(_ context.Context, key, value string, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.m[key] = value
	return nil
}

func (m *mailKV) IncrCtx(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	n := 1
	if v := m.m[key]; v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed + 1
		}
	}
	m.m[key] = strconv.Itoa(n)
	return int64(n), nil
}

func (m *mailKV) ExpireCtx(context.Context, string, int) error { return nil }
func (m *mailKV) SetnxExCtx(_ context.Context, key, value string, _ int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	if _, ok := m.m[key]; ok {
		return false, nil
	}
	m.m[key] = value
	return true, nil
}
func (m *mailKV) TtlCtx(_ context.Context, key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[key]; !ok {
		return -2, nil
	}
	return 60, nil
}
func (m *mailKV) EvalCtx(_ context.Context, _ string, phoneKey string, args ...any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	return evalMapQuota(m.m, phoneKey, args), nil
}

func evalMapQuota(m map[string]string, phoneKey string, args []any) int64 {
	if len(args) < 6 {
		return -1
	}
	caps := [3]int{}
	for i := 0; i < 3; i++ {
		switch n := args[i].(type) {
		case int:
			caps[i] = n
		case int64:
			caps[i] = int(n)
		}
	}
	ipKey, _ := args[4].(string)
	globalKey, _ := args[5].(string)
	keys := []string{phoneKey, ipKey, globalKey}
	cur := func(key string) int {
		if key == "" {
			return 0
		}
		n, _ := strconv.Atoi(m[key])
		return n
	}
	for i := 0; i < 3; i++ {
		if caps[i] > 0 && cur(keys[i])+1 > caps[i] {
			return int64(i + 1)
		}
	}
	for i := 0; i < 3; i++ {
		if caps[i] > 0 && keys[i] != "" {
			m[keys[i]] = strconv.Itoa(cur(keys[i]) + 1)
		}
	}
	return 0
}

func (m *mailKV) DecrCtx(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
	return 0, nil
}
