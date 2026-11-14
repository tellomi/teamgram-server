package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/core"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (s *Service) UserGetBlockedPage(ctx context.Context, request *user.TLUserGetBlockedList) (*mtproto.Contacts_Blocked, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserGetBlockedPage(request)
}

func (s *Service) UserGetFallbackPhoto(ctx context.Context, request *user.TLUserGetProfilePhotos) (*mtproto.Photo, error) {
	c := core.New(ctx, s.svcCtx)
	return c.UserGetFallbackPhoto(request)
}
