// Package statusbook remembers who last received a user's online status and
// the expires that was sent. A later heartbeat reuses that list to slide
// userStatusOnline.expires and does not read contacts, privacy, or blocks
// again. Contact, privacy, and block changes delete the list. Redis is used
// when KV is configured. Otherwise the mark stays in this process.
package statusbook

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

const (
	// TTL bounds how long a missed invalidation can keep a list. A successful
	// push refreshes it. The online window is 300s, so a live heartbeat
	// rewrites the key before this expires.
	TTL = 600
	// MaxViewers bounds one user's cached audience. A larger list is not
	// stored; the next heartbeat loads it again instead of keeping it.
	MaxViewers = 512
)

const keyPrefix = "status_audience_v1:"

// Viewer is one cached recipient. Exact is the precise online or offline
// status. Otherwise the recipient gets the coarse bucket, and ByMe is set
// when that bucket exists because they hid their own last seen.
type Viewer struct {
	ID    int64 `json:"id"`
	Exact bool  `json:"exact,omitempty"`
	ByMe  bool  `json:"by_me,omitempty"`
}

// Record is the last successful fanout. HaveList is false when the list was
// never stored or was too large to keep. Pushed is the expires viewers were
// given; zero means there is no mark.
type Record struct {
	Pushed   int64    `json:"pushed,omitempty"`
	Viewers  []Viewer `json:"viewers,omitempty"`
	HaveList bool     `json:"have_list,omitempty"`
}

type store interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetexCtx(ctx context.Context, key, value string, seconds int) error
	DelCtx(ctx context.Context, keys ...string) (int, error)
}

// Book is the shared last-audience mark.
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

// Load returns the stored audience. A missing key is a zero record and a nil
// error. A store error is returned so the caller still fans out, treating
// the list as unknown instead of skipping the renewal.
func (b *Book) Load(ctx context.Context, userID int64) (Record, error) {
	if b == nil || b.kv == nil || userID == 0 {
		return Record{}, nil
	}
	raw, err := b.kv.GetCtx(ctx, key(userID))
	if err != nil || raw == "" {
		return Record{}, err
	}
	var rec Record
	if json.Unmarshal([]byte(raw), &rec) != nil {
		return Record{}, nil
	}
	return rec, nil
}

// Save stores a newer expires and its audience. An older or equal expires
// is left untouched so a slow heartbeat cannot overwrite a later one. A
// list past MaxViewers is dropped and HaveList is false. A store error is
// returned; the push itself has already happened.
func (b *Book) Save(ctx context.Context, userID int64, rec Record) error {
	if b == nil || b.kv == nil || userID == 0 || rec.Pushed <= 0 {
		return nil
	}
	if len(rec.Viewers) > MaxViewers {
		rec.Viewers = nil
		rec.HaveList = false
	}
	cur, err := b.Load(ctx, userID)
	if err == nil && cur.Pushed >= rec.Pushed {
		return nil
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return b.kv.SetexCtx(ctx, key(userID), string(raw), TTL)
}

// Forget drops marks after a contact, privacy, or block change so the next
// heartbeat loads who may see the status.
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

// OpenRecord is the record a heartbeat may trust. A read error yields an
// empty record so the renewal still runs.
func OpenRecord(rec Record, err error) Record {
	if err != nil {
		return Record{}
	}
	return rec
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
