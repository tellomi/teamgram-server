package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountCancelPasswordEmail
// account.cancelPasswordEmail#c1cbd5b6 = Bool;
// https://core.telegram.org/method/account.cancelPasswordEmail
func (c *TwoFaCore) AccountCancelPasswordEmail(_ *mtproto.TLAccountCancelPasswordEmail) (*mtproto.Bool, error) {
	if c.accountPassword().GetEmailUnconfirmedPattern() == nil {
		c.Logger.Errorf("account.cancelPasswordEmail - error: %v", mtproto.ErrEmailHashExpired)
		return nil, mtproto.ErrEmailHashExpired
	}
	return mtproto.BoolTrue, nil
}
