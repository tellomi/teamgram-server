// Package sendas stores the default peer used when sending into a group.
//
// https://core.telegram.org/method/messages.saveDefaultSendAs
package sendas

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/teamgram/marmota/pkg/stores/kv"
	gokv "github.com/zeromicro/go-zero/core/stores/kv"
)

const keyPrefix = "default_send_as_v1:"

// Choice is the peer messages should be sent as.
type Choice struct {
	Type int32 `json:"type"`
	ID   int64 `json:"id"`
}

// Book stores one choice per user and destination group.
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

// Shared is the book saveDefaultSendAs writes and channels.getSendAs reads.
func Shared() *Book {
	mu.Lock()
	defer mu.Unlock()
	return shared
}

// Install replaces the process-wide book.
func Install(b *Book) {
	if b == nil {
		b = New(nil)
	}
	mu.Lock()
	shared = b
	mu.Unlock()
}

// Set stores the default send-as peer for one group.
func (b *Book) Set(ctx context.Context, user int64, peerType int32, peerID int64, choice Choice) error {
	raw, err := json.Marshal(choice)
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, key(user, peerType, peerID), string(raw))
}

// Get returns the saved default. ok is false when nothing is stored.
func (b *Book) Get(ctx context.Context, user int64, peerType int32, peerID int64) (Choice, bool) {
	raw, err := b.kv.GetCtx(ctx, key(user, peerType, peerID))
	if err != nil || raw == "" {
		return Choice{}, false
	}
	var choice Choice
	if json.Unmarshal([]byte(raw), &choice) != nil || choice.ID == 0 {
		return Choice{}, false
	}
	return choice, true
}

func key(user int64, peerType int32, peerID int64) string {
	return keyPrefix + strconv.FormatInt(user, 10) + ":" + strconv.FormatInt(int64(peerType), 10) + ":" + strconv.FormatInt(peerID, 10)
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
