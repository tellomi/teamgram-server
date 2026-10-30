package ephemeral_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/svc"
)

// New builds the ephemeral RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
