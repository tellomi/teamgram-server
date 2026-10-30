package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountGetReactionsNotifySettings
// account.getReactionsNotifySettings#6dd654c = ReactionsNotifySettings;
// https://core.telegram.org/method/account.getReactionsNotifySettings
// https://core.telegram.org/constructor/reactionsNotifySettings
func (c *NotificationCore) AccountGetReactionsNotifySettings(_ *mtproto.TLAccountGetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	if c.svcCtx == nil || c.svcCtx.Reactions == nil || c.MD == nil || c.MD.UserId == 0 {
		return defaultReactionsNotifySettings(), nil
	}
	raw, ok, err := c.svcCtx.Reactions.Load(c.ctx, c.MD.UserId)
	if err != nil || !ok || raw == "" {
		if err != nil {
			c.Logger.Errorf("account.getReactionsNotifySettings - error: %v", err)
		}
		return defaultReactionsNotifySettings(), nil
	}
	settings, err := decodeReactions(raw)
	if err != nil {
		c.Logger.Errorf("account.getReactionsNotifySettings - error: %v", err)
		return defaultReactionsNotifySettings(), nil
	}
	return settings, nil
}
