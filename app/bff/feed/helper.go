package feed_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/feed/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/feed/internal/svc"
)

// New builds the feed RPC service.
func New() *service.Service {
	return service.New(svc.New())
}
