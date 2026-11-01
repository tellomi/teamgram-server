// Package book stores Telegram Passport values for one BFF process.
//
// Redis is used when KV is configured. An in-process map is used otherwise,
// which is what the unit tests and a passport process without Redis get.
// The ciphertext is the client's. This server does not decrypt it: there is
// no passport private key, and account.getPassword does not persist a cloud
// password, so PASSWORD_REQUIRED is not raised here. A missing passport
// secret (secure_secret_id == 0) is SECURE_SECRET_REQUIRED.
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
}

// Book is the passport store shared by the passport handlers.
type Book struct {
	kv    store
	now   func() time.Time
	mu    sync.Mutex
	users map[int64]*userBox
}

// New uses Redis when conf is set and an in-process map otherwise.
func New(conf gokv.KvConf) *Book {
	var s store
	if len(conf) == 0 {
		s = newMem()
	} else {
		s = kv.NewStore(conf)
	}
	return &Book{
		kv:    s,
		now:   time.Now,
		users: map[int64]*userBox{},
	}
}

var (
	sharedMu sync.Mutex
	shared   = New(nil)
)

// Shared is the process-wide book.
func Shared() *Book {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// Install replaces the process-wide book. Passport startup calls it after
// opening Redis.
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
