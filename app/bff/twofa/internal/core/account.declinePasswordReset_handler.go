package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountDeclinePasswordReset
// account.declinePasswordReset#4c9409f6 = Bool;
// https://core.telegram.org/method/account.declinePasswordReset
func (c *TwoFaCore) AccountDeclinePasswordReset(_ *mtproto.TLAccountDeclinePasswordReset) (*mtproto.Bool, error) {
	if err := password.DeclineReset(c.ctx, c.userID()); err != nil {
		c.Logger.Errorf("account.declinePasswordReset - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
