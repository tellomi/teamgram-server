package core

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/verify"
)

func TestChangePhoneRejectsForeignAndEleventhBeforeSMS(t *testing.T) {
	kv := &quotaKV{}
	lim := verify.NewLimiter(kv, verify.Limits{ResendSeconds: 60, DailyPhone: 1})
	verify.UseSharedLimiters(lim, nil)
	t.Cleanup(func() { verify.UseSharedLimiters(nil, nil) })

	c := &AccountCore{ctx: context.Background()}
	if err := guardChangePhone(c, "14155552671"); err == nil || !strings.Contains(err.Error(), "PHONE_NUMBER_INVALID") {
		t.Fatalf("foreign: %v", err)
	}
	phone := "8613800138000"
	if err := guardChangePhone(c, phone); err != nil {
		t.Fatal(err)
	}
	kv.del("tellomi:code:cd:sms:" + phone)
	err := guardChangePhone(c, phone)
	if err == nil || !strings.Contains(err.Error(), "PHONE_NUMBER_FLOOD") {
		t.Fatalf("eleventh: %v", err)
	}
}

type quotaKV struct {
	mu sync.Mutex
	m  map[string]string
}

func (m *quotaKV) del(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
}

func (m *quotaKV) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		return "", nil
	}
	return m.m[key], nil
}

func (m *quotaKV) SetexCtx(_ context.Context, key, value string, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.m[key] = value
	return nil
}

func (m *quotaKV) IncrCtx(_ context.Context, key string) (int64, error) {
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

func (m *quotaKV) ExpireCtx(context.Context, string, int) error { return nil }
func (m *quotaKV) SetnxExCtx(_ context.Context, key, value string, _ int) (bool, error) {
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
func (m *quotaKV) TtlCtx(_ context.Context, key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[key]; !ok {
		return -2, nil
	}
	return 60, nil
}
func (m *quotaKV) DecrCtx(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
	return 0, nil
}
