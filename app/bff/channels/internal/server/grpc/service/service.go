package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/logx"
)

type Service struct {
	mtproto.UnimplementedRPCChannelsServer
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) users(ctx context.Context, self int64, ids ...int64) []*mtproto.User {
	if s.svcCtx.UserClient == nil || len(ids) == 0 {
		return []*mtproto.User{}
	}
	users, err := s.svcCtx.UserClient.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
		Id: ids,
		To: []int64{self},
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("channel users: %v", err)
		return []*mtproto.User{}
	}
	return users.GetUserListByIdList(self, ids...)
}
