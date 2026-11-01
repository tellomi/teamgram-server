// Package codelogin implements the P0 login, email, and phone-confirmation
// RPCs that official Telegram clients call. Codes are delivered by the
// existing SMS and email senders. Records live in redis when a KV is set,
// and in memory otherwise.
package codelogin

import (
	"context"
	"sync"
	"time"
)

// KV is the subset of the BFF redis client these records need.
type KV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

type memItem struct {
	v   string
	exp time.Time
}

type memKV struct {
	mu  sync.Mutex
	m   map[string]memItem
	now func() time.Time
}

func newMemKV(now func() time.Time) *memKV {
	if now == nil {
		now = time.Now
	}
	return &memKV{m: map[string]memItem{}, now: now}
}

func (m *memKV) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.m[key]
	if !ok || m.now().After(it.exp) {
		delete(m.m, key)
		return "", nil
	}
	return it.v, nil
}

func (m *memKV) SetexCtx(_ context.Context, key, value string, seconds int) error {
	if seconds < 1 {
		seconds = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = memItem{v: value, exp: m.now().Add(time.Duration(seconds) * time.Second)}
	return nil
}

func (m *memKV) DelCtx(_ context.Context, keys ...string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, key := range keys {
		if _, ok := m.m[key]; ok {
			delete(m.m, key)
			n++
		}
	}
	return n, nil
}

// PhoneCode is one pending auth.sendCode / auth.resendCode transaction.
type PhoneCode struct {
	AuthKeyID    int64
	Phone        string
	Hash         string
	Code         string
	Extra        string
	Expires      int64
	SentType     int
	EmailPattern string
	UserID       int64
	Registered   bool
}

// PhoneCodes is the existing phone-code record (redis, keyed by auth key and phone).
type PhoneCodes interface {
	Get(ctx context.Context, authKey int64, phone, hash string) (*PhoneCode, error)
	Save(ctx context.Context, code *PhoneCode) error
	Delete(ctx context.Context, authKey int64, phone, hash string) error
}

// MemPhones is an in-process phone-code table for tests.
type MemPhones struct {
	mu sync.Mutex
	m  map[string]*PhoneCode
}

func NewMemPhones() *MemPhones {
	return &MemPhones{m: map[string]*PhoneCode{}}
}

func phoneKey(authKey int64, phone string) string {
	return string(rune(0)) + itoa(authKey) + "\t" + phone
}

func (p *MemPhones) Put(code *PhoneCode) {
	if code == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := *code
	p.m[phoneKey(code.AuthKeyID, code.Phone)] = &cp
}

func (p *MemPhones) Get(_ context.Context, authKey int64, phone, hash string) (*PhoneCode, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.m[phoneKey(authKey, phone)]
	if cur == nil || cur.Hash != hash {
		return nil, nil
	}
	cp := *cur
	return &cp, nil
}

func (p *MemPhones) Save(_ context.Context, code *PhoneCode) error {
	p.Put(code)
	return nil
}

func (p *MemPhones) Delete(_ context.Context, authKey int64, phone, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, phoneKey(authKey, phone))
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
