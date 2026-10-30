package payments_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/svc"
)

// New builds the Stars, subscription, giveaway, and gift-code RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
