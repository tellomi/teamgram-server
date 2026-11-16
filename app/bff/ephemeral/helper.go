package ephemeral_helper

import (
	"context"

	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/svc"
)

// New builds the ephemeral RPC service.
func New() *service.Service {
	return service.New(svc.New())
}

// Start installs the bot lookup. ephemeral.* is bot-only; a user whose
// session metadata does not carry is_bot is checked here.
func Start(check func(context.Context, int64) (bool, error)) {
	svc.SetBotCheck(check)
}
