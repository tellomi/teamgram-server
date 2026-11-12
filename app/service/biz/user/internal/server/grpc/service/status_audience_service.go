package service

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/core"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (s *Service) UserGetReverseContactIdList(ctx context.Context, request *user.TLUserGetContactIdList) (*user.Vector_Long, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserGetReverseContactIdList(request)
}

func (s *Service) UserBlockedEitherWay(ctx context.Context, request *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserBlockedEitherWay(request)
}
