package webpage

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// UserCard is the public profile used for a telegram_user page.
type UserCard struct {
	ID    int64
	Title string
	About string
	User  *mtproto.User
}

// UserResolver looks up a public username. ok is false when the name is
// not a user. A lookup error is ok false, not a failed RPC for the caller.
type UserResolver func(ctx context.Context, username string) (UserCard, bool)

var (
	userMu     sync.RWMutex
	lookupUser UserResolver
)

// BindUserResolver installs the user directory used for telegram_user cards.
// The messages service calls this at startup. Tests replace it.
func BindUserResolver(fn UserResolver) {
	userMu.Lock()
	lookupUser = fn
	userMu.Unlock()
}

func lookupUserCard(ctx context.Context, username string) (UserCard, bool) {
	userMu.RLock()
	fn := lookupUser
	userMu.RUnlock()
	if fn == nil {
		return UserCard{}, false
	}
	return fn(ctx, username)
}
