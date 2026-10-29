package stories_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/svc"
)

type Config = config.Config

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
