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
	var data *mtproto.Help_PromoData
	if c.svcCtx == nil || c.svcCtx.Promo == nil {
		data = promo.Empty()
	} else {
		data = c.svcCtx.Promo.Get(c.ctx, userID)
	}
	return c.withSuggestions(data, userID), nil
}

// withSuggestions copies pending and dismissed suggestion names onto promo data
// when this user has any. help.promoDataEmpty has no such fields, so a
// non-empty list is returned as help.promoData without a sponsor peer.
func (c *ConfigurationCore) withSuggestions(data *mtproto.Help_PromoData, userID int64) *mtproto.Help_PromoData {
	if c.svcCtx == nil || c.svcCtx.Suggestions == nil || data == nil {
		return data
	}
	pending, dismissed := c.svcCtx.Suggestions.Lists(c.ctx, userID, mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer())
	if len(pending) == 0 && len(dismissed) == 0 {
		return data
	}
	if data.GetPredicateName() == mtproto.Predicate_help_promoDataEmpty {
		return mtproto.MakeTLHelpPromoData(&mtproto.Help_PromoData{
			Expires:              data.GetExpires(),
			PendingSuggestions:   pending,
			DismissedSuggestions: dismissed,
			Chats:                []*mtproto.Chat{},
			Users:                []*mtproto.User{},
		}).To_Help_PromoData()
	}
	data.PendingSuggestions = pending
	data.DismissedSuggestions = dismissed
	return data
}
