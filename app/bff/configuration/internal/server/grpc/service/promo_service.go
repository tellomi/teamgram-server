package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/core"
)

// HelpGetPromoData
// help.getPromoData#c0977421 = help.PromoData;
func (s *Service) HelpGetPromoData(ctx context.Context, request *mtproto.TLHelpGetPromoData) (*mtproto.Help_PromoData, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("help.getPromoData - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.HelpGetPromoData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("help.getPromoData - reply: {%s}", r)
	return r, err
}

// HelpHidePromoData
// help.hidePromoData#1e251c95 peer:InputPeer = Bool;
func (s *Service) HelpHidePromoData(ctx context.Context, request *mtproto.TLHelpHidePromoData) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("help.hidePromoData - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.HelpHidePromoData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("help.hidePromoData - reply: {%s}", r)
	return r, err
}

var _ mtproto.RPCPromoDataServer = (*Service)(nil)
