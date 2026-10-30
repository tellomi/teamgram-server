package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/core"
)

// AccountGetReactionsNotifySettings
// account.getReactionsNotifySettings#6dd654c = ReactionsNotifySettings;
func (s *Service) AccountGetReactionsNotifySettings(ctx context.Context, request *mtproto.TLAccountGetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("account.getReactionsNotifySettings - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.AccountGetReactionsNotifySettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("account.getReactionsNotifySettings - reply: {%s}", r)
	return r, err
}

// AccountSetReactionsNotifySettings
// account.setReactionsNotifySettings#316ce548 settings:ReactionsNotifySettings = ReactionsNotifySettings;
func (s *Service) AccountSetReactionsNotifySettings(ctx context.Context, request *mtproto.TLAccountSetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("account.setReactionsNotifySettings - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.AccountSetReactionsNotifySettings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("account.setReactionsNotifySettings - reply: {%s}", r)
	return r, err
}
