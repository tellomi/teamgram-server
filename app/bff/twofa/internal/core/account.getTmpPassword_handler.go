package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountGetTmpPassword
// account.getTmpPassword#449e0b51 password:InputCheckPasswordSRP period:int = account.TmpPassword;
// https://core.telegram.org/method/account.getTmpPassword
// https://core.telegram.org/constructor/account.tmpPassword
func (c *TwoFaCore) AccountGetTmpPassword(in *mtproto.TLAccountGetTmpPassword) (*mtproto.Account_TmpPassword, error) {
	var (
		proof  *mtproto.InputCheckPasswordSRP
		period int32
	)
	if in != nil {
		proof = in.GetPassword()
		period = in.GetPeriod()
	}
	has, err := password.HasPassword(c.ctx, c.userID())
	if err != nil {
		c.Logger.Errorf("account.getTmpPassword - error: %v", err)
		return nil, err
	}
	if !has {
		if err = c.checkTmpPassword(proof); err != nil {
			c.Logger.Errorf("account.getTmpPassword - error: %v", err)
			return nil, err
		}
		return c.makeTmpPassword(clampTmpPeriod(period)), nil
	}
	if len(proof.GetA()) == 0 {
		c.Logger.Errorf("account.getTmpPassword - error: %v", errSRPAInvalid)
		return nil, errSRPAInvalid
	}
	if err = password.Verify(c.ctx, c.userID(), c.authKey(), proof); err != nil {
		c.Logger.Errorf("account.getTmpPassword - error: %v", err)
		return nil, err
	}
	return c.makeTmpPassword(clampTmpPeriod(period)), nil
}
