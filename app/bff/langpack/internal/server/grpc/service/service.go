package service

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/svc"
)

type Service struct {
	mtproto.UnimplementedRPCLangpackServer
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) GetServiceContext() *svc.ServiceContext {
	return s.svcCtx
}
