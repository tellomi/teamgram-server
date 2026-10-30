package core

import (
	"github.com/teamgram/proto/mtproto"
)

// HelpHidePromoData
// help.hidePromoData#1e251c95 peer:InputPeer = Bool;
// https://core.telegram.org/method/help.hidePromoData
// https://core.telegram.org/api/config#suggestions
func (c *ConfigurationCore) HelpHidePromoData(in *mtproto.TLHelpHidePromoData) (*mtproto.Bool, error) {
	var (
		userID int64
		peer   *mtproto.InputPeer
	)
	if c.MD != nil {
		userID = c.MD.UserId
	}
	if in != nil {
		peer = in.GetPeer()
	}
	if c.svcCtx == nil || c.svcCtx.Promo == nil {
		c.Logger.Errorf("help.hidePromoData - error: promo store is not configured")
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := c.svcCtx.Promo.Hide(c.ctx, userID, peer); err != nil {
		c.Logger.Errorf("help.hidePromoData - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
