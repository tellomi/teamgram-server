// Package botconn stores account.confirmBotConnection.
// https://core.telegram.org/method/account.confirmBotConnection
//
// The method returns Bool. A confirmed pair is remembered in this process
// and, when a cache is passed, under botconn:<user>:<bot>. Nothing here
// flushes Redis.
package botconn

import (
	"context"
	"strconv"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// KV is the subset of the account cache these rows use.
type KV interface {
	GetCtx(ctx context.Context, key string) (string, error)
	SetCtx(ctx context.Context, key, value string) error
}

var (
	mu   sync.Mutex
	seen = map[string]struct{}{}
)

// Reset drops process memory. Tests call it. It does not touch Redis.
func Reset() {
	mu.Lock()
	seen = map[string]struct{}{}
	mu.Unlock()
}

// Target reads bot_id. inputUserFromMessage and inputUserEmpty are
// BOT_ID_INVALID. inputUserSelf is the signed-in user.
func Target(self int64, in *mtproto.InputUser) (id, hash int64, err error) {
	if in == nil {
		return 0, 0, mtproto.ErrBotIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputUser:
		if in.GetUserId() <= 0 {
			return 0, 0, mtproto.ErrBotIdInvalid
		}
		return in.GetUserId(), in.GetAccessHash(), nil
	case mtproto.Predicate_inputUserSelf:
		if self <= 0 {
			return 0, 0, mtproto.ErrAuthKeyUnregistered
		}
		return self, 0, nil
	default:
		return 0, 0, mtproto.ErrBotIdInvalid
	}
}

// Confirm remembers that self confirmed bot. Calling it again stays true.
func Confirm(ctx context.Context, self, bot int64, kv KV) (*mtproto.Bool, error) {
	if self <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if bot <= 0 {
		return nil, mtproto.ErrBotIdInvalid
	}
	key := rowKey(self, bot)
	mu.Lock()
	seen[key] = struct{}{}
	mu.Unlock()
	if kv != nil {
		_ = kv.SetCtx(ctx, key, "1")
	}
	return mtproto.BoolTrue, nil
}

// Confirmed reports a pair recorded by Confirm in this process or in kv.
func Confirmed(ctx context.Context, self, bot int64, kv KV) bool {
	key := rowKey(self, bot)
	mu.Lock()
	_, ok := seen[key]
	mu.Unlock()
	if ok {
		return true
	}
	if kv == nil {
		return false
	}
	v, err := kv.GetCtx(ctx, key)
	return err == nil && v == "1"
}

func rowKey(self, bot int64) string {
	return "botconn:" + strconv.FormatInt(self, 10) + ":" + strconv.FormatInt(bot, 10)
}
