package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountDeclinePasswordReset
// account.declinePasswordReset#4c9409f6 = Bool;
// https://core.telegram.org/method/account.declinePasswordReset
func (c *TwoFaCore) AccountDeclinePasswordReset(_ *mtproto.TLAccountDeclinePasswordReset) (*mtproto.Bool, error) {
	if c.accountPassword().GetPendingResetDate() == nil {
		c.Logger.Errorf("account.declinePasswordReset - error: %v", mtproto.ErrResetRequestMissing)
		return nil, mtproto.ErrResetRequestMissing
	}
	return mtproto.BoolTrue, nil
}
