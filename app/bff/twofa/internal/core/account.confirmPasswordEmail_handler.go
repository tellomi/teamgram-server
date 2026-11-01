package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountConfirmPasswordEmail
// account.confirmPasswordEmail#8fdf1920 code:string = Bool;
// https://core.telegram.org/method/account.confirmPasswordEmail
func (c *TwoFaCore) AccountConfirmPasswordEmail(in *mtproto.TLAccountConfirmPasswordEmail) (*mtproto.Bool, error) {
	code := ""
	if in != nil {
		code = in.Code
	}
	if err := password.ConfirmEmail(c.ctx, c.userID(), code); err != nil {
		c.Logger.Errorf("account.confirmPasswordEmail - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
