package wallet_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/svc"
)

// New builds the wallet RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
