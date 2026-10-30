package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountSetReactionsNotifySettings
// account.setReactionsNotifySettings#316ce548 settings:ReactionsNotifySettings = ReactionsNotifySettings;
// https://core.telegram.org/method/account.setReactionsNotifySettings
// https://core.telegram.org/constructor/reactionsNotifySettings
func (c *NotificationCore) AccountSetReactionsNotifySettings(in *mtproto.TLAccountSetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	var incoming *mtproto.ReactionsNotifySettings
	if in != nil {
		incoming = in.GetSettings()
	}
	settings := normalizeReactionsNotifySettings(incoming)
	if c.svcCtx == nil || c.svcCtx.Reactions == nil || c.MD == nil || c.MD.UserId == 0 {
		return settings, nil
	}
	raw, err := encodeReactions(settings)
	if err != nil {
		c.Logger.Errorf("account.setReactionsNotifySettings - error: %v", err)
		return nil, err
	}
	if err = c.svcCtx.Reactions.Save(c.ctx, c.MD.UserId, raw); err != nil {
		c.Logger.Errorf("account.setReactionsNotifySettings - error: %v", err)
		return nil, err
	}
	return settings, nil
}
