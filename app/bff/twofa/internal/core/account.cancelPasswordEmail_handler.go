package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountCancelPasswordEmail
// account.cancelPasswordEmail#c1cbd5b6 = Bool;
// https://core.telegram.org/method/account.cancelPasswordEmail
func (c *TwoFaCore) AccountCancelPasswordEmail(_ *mtproto.TLAccountCancelPasswordEmail) (*mtproto.Bool, error) {
	if err := password.CancelEmail(c.ctx, c.userID()); err != nil {
		c.Logger.Errorf("account.cancelPasswordEmail - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
