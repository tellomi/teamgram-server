package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/logx"
)

type Service struct {
	mtproto.UnimplementedRPCStoriesServer
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) md(ctx context.Context) (*metadata.RpcMetadata, error) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil || md.GetUserId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	return md, nil
}

func (s *Service) users(ctx context.Context, self int64, ids ...int64) []*mtproto.User {
	if s.svcCtx == nil || s.svcCtx.UserClient == nil || len(ids) == 0 {
		return []*mtproto.User{}
	}
	seen := map[int64]bool{}
	var uniq []int64
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return []*mtproto.User{}
	}
	users, err := s.svcCtx.UserClient.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
		Id: uniq,
		To: []int64{self},
	})
	if err != nil || users == nil {
		logx.WithContext(ctx).Errorf("story users: %v", err)
		return []*mtproto.User{}
	}
	return users.GetUserListByIdList(self, uniq...)
}
