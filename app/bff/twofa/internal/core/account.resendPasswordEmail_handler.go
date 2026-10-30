package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountResendPasswordEmail
// account.resendPasswordEmail#7a7f2a15 = Bool;
// https://core.telegram.org/method/account.resendPasswordEmail
func (c *TwoFaCore) AccountResendPasswordEmail(_ *mtproto.TLAccountResendPasswordEmail) (*mtproto.Bool, error) {
	if c.accountPassword().GetEmailUnconfirmedPattern() == nil {
		c.Logger.Errorf("account.resendPasswordEmail - error: %v", mtproto.ErrEmailHashExpired)
		return nil, mtproto.ErrEmailHashExpired
	}
	return mtproto.BoolTrue, nil
}
