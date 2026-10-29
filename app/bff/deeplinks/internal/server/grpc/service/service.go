package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/svc"
)

type Service struct {
	mtproto.UnimplementedRPCDeepLinksServer
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) HelpGetDeepLinkInfo(ctx context.Context, request *mtproto.TLHelpGetDeepLinkInfo) (*mtproto.Help_DeepLinkInfo, error) {
	return core.New(ctx, s.svcCtx).HelpGetDeepLinkInfo(request)
}

func (s *Service) HelpGetRecentMeUrls(ctx context.Context, request *mtproto.TLHelpGetRecentMeUrls) (*mtproto.Help_RecentMeUrls, error) {
	return core.New(ctx, s.svcCtx).HelpGetRecentMeUrls(request)
}
