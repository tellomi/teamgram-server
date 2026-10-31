package smsjobs_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/smsjobs/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/smsjobs/internal/svc"
)

// New builds the smsjobs RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
