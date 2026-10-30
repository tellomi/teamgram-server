package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/promo"
)

// HelpGetPromoData
// help.getPromoData#c0977421 = help.PromoData;
// https://core.telegram.org/method/help.getPromoData
// https://core.telegram.org/constructor/help.promoDataEmpty
// https://core.telegram.org/api/config#suggestions
func (c *ConfigurationCore) HelpGetPromoData(_ *mtproto.TLHelpGetPromoData) (*mtproto.Help_PromoData, error) {
	var userID int64
	if c.MD != nil {
		userID = c.MD.UserId
	}
	if c.svcCtx == nil || c.svcCtx.Promo == nil {
		return promo.Empty(), nil
	}
	return c.svcCtx.Promo.Get(c.ctx, userID), nil
}
