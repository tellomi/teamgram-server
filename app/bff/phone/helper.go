package phone_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/svc"
)

type (
	Config  = config.Config
	Clients = svc.Clients
)

func New(c Config, clients Clients) *service.Service {
	return service.New(svc.New(c, clients))
}
