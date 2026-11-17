package svc

import (
	"context"
	"sync"

	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/box"
)

// ServiceContext holds the process-local ephemeral box.
type ServiceContext struct {
	Box *box.Box
}

// New returns a context bound to the shared box.
func New() *ServiceContext {
	return &ServiceContext{Box: box.Shared()}
}

var (
	botMu sync.Mutex
	botFn func(context.Context, int64) (bool, error)
)

// SetBotCheck installs the user-service lookup used when session metadata
// does not already say the caller is a bot. Nil means only metadata and the
// built-in bot ids count.
func SetBotCheck(fn func(context.Context, int64) (bool, error)) {
	botMu.Lock()
	botFn = fn
	botMu.Unlock()
}

// BotCheck is the lookup installed by SetBotCheck.
func BotCheck() func(context.Context, int64) (bool, error) {
	botMu.Lock()
	defer botMu.Unlock()
	return botFn
}
