package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountGetTmpPassword
// account.getTmpPassword#449e0b51 password:InputCheckPasswordSRP period:int = account.TmpPassword;
// https://core.telegram.org/method/account.getTmpPassword
// https://core.telegram.org/constructor/account.tmpPassword
func (c *TwoFaCore) AccountGetTmpPassword(in *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	var (
		password *mtproto.InputCheckPasswordSRP
		period   int32
	)
	if in != nil {
		password = in.GetPassword()
		period = in.GetPeriod()
	}
	if err := c.checkTmpPassword(password); err != nil {
		c.Logger.Errorf("account.getTmpPassword - error: %v", err)
		return nil, err
	}
	return c.makeTmpPassword(clampTmpPeriod(period)), nil
}
