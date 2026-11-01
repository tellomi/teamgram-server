// Package notifyack remembers message ids that were handed to push delivery.
//
// messages.receivedMessages confirms the client has them and cancels further
// pushes for those ids.
// https://core.telegram.org/method/messages.receivedMessages
package notifyack

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"sync"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

const (
	keyPrefix  = "notify_ack_v1:"
	maxPending = 200
)

// Book is the pending-push list for one process, backed by Redis when configured.
type Book struct {
	kv store
}

type store interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetCtx(ctx context.Context, key, value string) error
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
	mu     sync.Mutex
	shared = New(nil)
)

// Shared is the book sync.pushUpdates writes and messages.receivedMessages reads.
func Shared() *Book {
	mu.Lock()
	defer mu.Unlock()
	return shared
}

// Install replaces the process-wide book. Sync and the messages BFF both call
// it with the same Redis so an ack survives the process boundary.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	mu.Lock()
	shared = b
	mu.Unlock()
}

// Note records message ids that were offered to push.
func (b *Book) Note(ctx context.Context, user int64, ids []int32) error {
	if b == nil || user == 0 || len(ids) == 0 {
		return nil
	}
	cur := b.load(ctx, user)
	seen := map[int32]bool{}
	for _, id := range cur {
		seen[id] = true
	}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		cur = append(cur, id)
	}
	if len(cur) > maxPending {
		cur = cur[len(cur)-maxPending:]
	}
	return b.save(ctx, user, cur)
}

// Take returns and drops every pending id at or below maxID.
func (b *Book) Take(ctx context.Context, user int64, maxID int32) ([]int32, error) {
	if b == nil || user == 0 || maxID <= 0 {
		return nil, nil
	}
	cur := b.load(ctx, user)
	var taken, keep []int32
	for _, id := range cur {
		if id <= maxID {
			taken = append(taken, id)
			continue
		}
		keep = append(keep, id)
	}
	if len(taken) == 0 {
		return nil, nil
	}
	sort.Slice(taken, func(i, j int) bool { return taken[i] < taken[j] })
	if err := b.save(ctx, user, keep); err != nil {
		return nil, err
	}
	return taken, nil
}

func (b *Book) load(ctx context.Context, user int64) []int32 {
	raw, err := b.kv.GetCtx(ctx, keyPrefix+strconv.FormatInt(user, 10))
	if err != nil || raw == "" {
		return nil
	}
	var ids []int32
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return nil
	}
	return ids
}

func (b *Book) save(ctx context.Context, user int64, ids []int32) error {
	if len(ids) == 0 {
		return b.kv.SetCtx(ctx, keyPrefix+strconv.FormatInt(user, 10), "[]")
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, keyPrefix+strconv.FormatInt(user, 10), string(raw))
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
