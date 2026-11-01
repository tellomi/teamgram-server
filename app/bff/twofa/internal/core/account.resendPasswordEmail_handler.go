package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountResendPasswordEmail
// account.resendPasswordEmail#7a7f2a15 = Bool;
// https://core.telegram.org/method/account.resendPasswordEmail
func (c *TwoFaCore) AccountResendPasswordEmail(_ *mtproto.TLAccountResendPasswordEmail) (*mtproto.Bool, error) {
	if err := password.ResendEmail(c.ctx, c.userID()); err != nil {
		c.Logger.Errorf("account.resendPasswordEmail - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
