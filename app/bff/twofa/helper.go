package twofa_helper

import (
	"github.com/teamgram/teamgram-server/app/bff/twofa/internal/server/grpc/service"
)

func New() *service.Service {
	return service.New()
}
