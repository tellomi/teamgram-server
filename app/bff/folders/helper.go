package folders_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/svc"
)

type (
	Config = config.Config
)

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
