package communities_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/svc"
)

// New builds the communities RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
