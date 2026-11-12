// Package statusbook remembers the userStatusOnline.expires last pushed for
// a user. Heartbeats consult it so a renewal does not reload the audience
// until that deadline is close to running out. Contact, privacy, and block
// changes delete the mark; the next heartbeat then loads a fresh audience.
// Redis is used when KV is configured. Otherwise the mark stays in this
// process, which is what unit tests and a single BFF get.
package statusbook

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

// TTL bounds a missed invalidation. A foreground renewal happens well
// inside this window, and a relationship change deletes the mark itself.
const TTL = 600

const keyPrefix = "status_pushed_expires_v1:"

type store interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

// Book is the last pushed expires for online status fanout.
type Book struct {
	kv store
}

// New uses Redis when conf is set and an in-process map otherwise.
func New(conf gokv.KvConf) *Book {
	var s store
	if len(conf) == 0 {
		s = newMem()
	} else {
		s = kv.NewStore(conf)
	}
	return &Book{kv: s}
}

var (
	sharedMu sync.Mutex
	shared   = New(nil)
)

// Shared is the book account.updateStatus and the contact/privacy handlers share.
func Shared() *Book {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// Install replaces the process-wide book. Startup calls it after opening
// Redis. Tests call it so cases do not leak into each other.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	sharedMu.Lock()
	shared = b
	sharedMu.Unlock()
}

func key(userID int64) string {
	return keyPrefix + strconv.FormatInt(userID, 10)
}

// Pushed returns the expires last sent to viewers. Zero means this process
// has no mark (never pushed, forgotten, or the TTL elapsed). A store error
// is returned to the caller; treating it as zero makes the next heartbeat
// load the audience instead of skipping it.
func (b *Book) Pushed(ctx context.Context, userID int64) (int64, error) {
	if b == nil || b.kv == nil || userID == 0 {
		return 0, nil
	}
	raw, err := b.kv.GetCtx(ctx, key(userID))
	if err != nil || raw == "" {
		return 0, err
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, nil
	}
	return n, nil
}

// Remember stores the expires just pushed. Zero forgets the mark.
func (b *Book) Remember(ctx context.Context, userID, expires int64) error {
	if b == nil || b.kv == nil || userID == 0 {
		return nil
	}
	if expires <= 0 {
		_, err := b.kv.DelCtx(ctx, key(userID))
		return err
	}
	return b.kv.SetexCtx(ctx, key(userID), strconv.FormatInt(expires, 10), TTL)
}

// Forget drops marks after a contact, privacy, or block change so the next
// heartbeat reloads who may see the status.
func (b *Book) Forget(ctx context.Context, userIDs ...int64) {
	if b == nil || b.kv == nil {
		return
	}
	keys := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id != 0 {
			keys = append(keys, key(id))
		}
	}
	if len(keys) == 0 {
		return
	}
	_, _ = b.kv.DelCtx(ctx, keys...)
}

type memVal struct {
	v      string
	expire time.Time
}

type mem struct {
	mu sync.Mutex
	m  map[string]memVal
}

func newMem() *mem { return &mem{m: map[string]memVal{}} }

func (m *mem) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ent, ok := m.m[key]
	if !ok {
		return "", nil
	}
	if !ent.expire.IsZero() && time.Now().After(ent.expire) {
		delete(m.m, key)
		return "", nil
	}
	return ent.v, nil
}

func (m *mem) SetexCtx(_ context.Context, key, value string, seconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = memVal{v: value, expire: time.Now().Add(time.Duration(seconds) * time.Second)}
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
