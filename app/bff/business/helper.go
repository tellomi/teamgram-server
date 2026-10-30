package business_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/business/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/business/internal/svc"
)

// New builds the business RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
