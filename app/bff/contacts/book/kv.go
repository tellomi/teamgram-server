// Package book stores contact notes, top-peer ratings, the story blocklist,
// and People Nearby points for one BFF process.
//
// Redis is used when KV is configured. An in-process map is used otherwise,
// which is what the unit tests and a contacts process without Redis get.
// users.getFullUser reads the same book so a saved note and a story block
// show up on userFull.
package book

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

type store interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetCtx(ctx context.Context, key, value string) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

// Book is the contact-relationship store.
type Book struct {
	kv  store
	now func() time.Time
}

// New uses Redis when conf is set and an in-process map otherwise.
func New(conf gokv.KvConf) *Book {
	var s store
	if len(conf) == 0 {
		s = newMem()
	} else {
		s = kv.NewStore(conf)
	}
	return &Book{kv: s, now: time.Now}
}

var (
	sharedMu sync.Mutex
	shared   = New(nil)
)

// Shared is the book users.getFullUser and the contacts handlers share.
func Shared() *Book {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// Install replaces the process-wide book. Contacts startup calls it after
// opening Redis. Tests call it so cases do not leak into each other.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	sharedMu.Lock()
	shared = b
	sharedMu.Unlock()
}

type mem struct {
	mu sync.Mutex
	m  map[string]string
}

func newMem() *mem { return &mem{m: map[string]string{}} }

func (m *mem) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[key], nil
}

func (m *mem) SetCtx(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = value
	return nil
}

func (m *mem) DelCtx(_ context.Context, keys ...string) (int, error) {
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
