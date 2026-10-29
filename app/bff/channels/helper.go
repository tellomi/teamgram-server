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

func NewForums() *service.ForumsService       { return &service.ForumsService{} }
func NewReactions() *service.ReactionsService { return &service.ReactionsService{} }
func NewThreads() *service.ThreadsService     { return &service.ThreadsService{} }
func NewStats() *service.StatsService         { return &service.StatsService{} }
