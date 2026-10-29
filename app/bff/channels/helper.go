package channels_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/svc"
)

type Config = config.Config

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
