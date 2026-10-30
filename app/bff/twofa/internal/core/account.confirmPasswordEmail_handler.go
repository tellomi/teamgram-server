package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountConfirmPasswordEmail
// account.confirmPasswordEmail#8fdf1920 code:string = Bool;
// https://core.telegram.org/method/account.confirmPasswordEmail
func (c *TwoFaCore) AccountConfirmPasswordEmail(_ *mtproto.TLAccountConfirmPasswordEmail) (*mtproto.Bool, error) {
	// No recovery email is waiting for a code, so the confirmation hash is gone.
	if c.accountPassword().GetEmailUnconfirmedPattern() == nil {
		c.Logger.Errorf("account.confirmPasswordEmail - error: %v", mtproto.ErrEmailHashExpired)
		return nil, mtproto.ErrEmailHashExpired
	}
	c.Logger.Errorf("account.confirmPasswordEmail - error: %v", mtproto.ErrCodeInvalid)
	return nil, mtproto.ErrCodeInvalid
}
