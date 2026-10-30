package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesToggleDialogFilterTags
// messages.toggleDialogFilterTags#fd2dda49 enabled:Bool = Bool;
// https://core.telegram.org/method/messages.toggleDialogFilterTags
// https://core.telegram.org/api/folders
//
// Enabling tags requires Premium (403 PREMIUM_ACCOUNT_REQUIRED).
// Disabling always succeeds. A value that is already stored does not emit
// updateDialogFilters. A change emits it to the user's other sessions.
func (c *FoldersCore) MessagesToggleDialogFilterTags(in *mtproto.TLMessagesToggleDialogFilterTags) (*mtproto.Bool, error) {
	enabled := mtproto.FromBool(in.GetEnabled())
	current, err := c.svcCtx.Dao.DialogClient.DialogGetDialogFilterTags(c.ctx, &dialog.TLDialogGetDialogFilterTags{
		UserId: c.userID(),
	})
	if err != nil {
		c.Logger.Errorf("messages.toggleDialogFilterTags - read error: %v", err)
		return nil, err
	}
	if mtproto.FromBool(current) == enabled {
		return mtproto.BoolTrue, nil
	}
	if enabled && !c.premiumMember() {
		return nil, mtproto.ErrForbiddenPremiumAccountRequired
	}
	if _, err = c.svcCtx.Dao.DialogClient.DialogToggleDialogFilterTags(c.ctx, &dialog.TLDialogToggleDialogFilterTags{
		UserId:  c.userID(),
		Enabled: mtproto.ToBool(enabled),
	}); err != nil {
		c.Logger.Errorf("messages.toggleDialogFilterTags - error: %v", err)
		return nil, err
	}
	c.pushNotMe(mtproto.MakeTLUpdateDialogFilters(&mtproto.Update{}).To_Update())
	return mtproto.BoolTrue, nil
}
