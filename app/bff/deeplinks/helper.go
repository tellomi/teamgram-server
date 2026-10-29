package deeplinks_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/svc"
)

type Config = config.Config

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
