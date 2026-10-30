package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

func (s *Service) FeedGetFeed(ctx context.Context, in *mtproto.TLFeedGetFeed) (*mtproto.Feed_FeedMessages, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Feed(id, in)
}

func (s *Service) FeedGetForYou(ctx context.Context, in *mtproto.TLFeedGetForYou) (*mtproto.Feed_ForYouFeed, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.ForYou(id, in)
}

func (s *Service) FeedReadFeed(ctx context.Context, in *mtproto.TLFeedReadFeed) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Read(id, in.GetFilterId(), in.GetMaxPosition())
}

func (s *Service) FeedGetInterestCategories(ctx context.Context, in *mtproto.TLFeedGetInterestCategories) (*mtproto.Feed_InterestCategories, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	hash := int64(0)
	if in != nil {
		hash = in.GetHash()
	}
	return s.svcCtx.Book.Interests(id, hash)
}

func (s *Service) FeedSetInterestCategories(ctx context.Context, in *mtproto.TLFeedSetInterestCategories) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var ids []int32
	if in != nil {
		ids = in.GetCategoryIds()
	}
	return s.svcCtx.Book.SetInterests(id, ids)
}
