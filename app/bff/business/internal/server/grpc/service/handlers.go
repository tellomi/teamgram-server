package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

func (s *Service) AccountUpdateBusinessWorkHours(ctx context.Context, in *mtproto.TLAccountUpdateBusinessWorkHours) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.SetWorkHours(id, in.GetBusinessWorkHours()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) AccountUpdateBusinessLocation(ctx context.Context, in *mtproto.TLAccountUpdateBusinessLocation) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.SetLocation(id, in.GetGeoPoint(), in.GetAddress()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) AccountUpdateBusinessGreetingMessage(ctx context.Context, in *mtproto.TLAccountUpdateBusinessGreetingMessage) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if msg := in.GetMessage(); msg != nil {
		if err := requireShortcut(ctx, id, msg.GetShortcutId()); err != nil {
			return nil, err
		}
	}
	if err := s.svcCtx.Book.SetGreeting(id, in.GetMessage()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) AccountUpdateBusinessAwayMessage(ctx context.Context, in *mtproto.TLAccountUpdateBusinessAwayMessage) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if msg := in.GetMessage(); msg != nil {
		if err := requireShortcut(ctx, id, msg.GetShortcutId()); err != nil {
			return nil, err
		}
	}
	if err := s.svcCtx.Book.SetAway(id, in.GetMessage()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) AccountUpdateBusinessIntro(ctx context.Context, in *mtproto.TLAccountUpdateBusinessIntro) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.SetIntro(id, in.GetIntro()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) AccountCreateBusinessChatLink(ctx context.Context, in *mtproto.TLAccountCreateBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.CreateLink(id, in.GetLink())
}

func (s *Service) AccountEditBusinessChatLink(ctx context.Context, in *mtproto.TLAccountEditBusinessChatLink) (*mtproto.BusinessChatLink, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.EditLink(id, in.GetSlug(), in.GetLink())
}

func (s *Service) AccountDeleteBusinessChatLink(ctx context.Context, in *mtproto.TLAccountDeleteBusinessChatLink) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.DeleteLink(id, in.GetSlug()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) AccountGetBusinessChatLinks(ctx context.Context, _ *mtproto.TLAccountGetBusinessChatLinks) (*mtproto.Account_BusinessChatLinks, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Links(id), nil
}

func (s *Service) AccountResolveBusinessChatLink(ctx context.Context, in *mtproto.TLAccountResolveBusinessChatLink) (*mtproto.Account_ResolvedBusinessChatLinks, error) {
	if _, err := s.user(ctx); err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Resolve(in.GetSlug())
}

func requireShortcut(ctx context.Context, user int64, shortcutID int32) error {
	if shortcutID <= 0 {
		return mtproto.ErrShortcutInvalid
	}
	_, err := chatbox.Shared().QuickReplyMessages(ctx, user, shortcutID, nil, 0)
	return err
}
