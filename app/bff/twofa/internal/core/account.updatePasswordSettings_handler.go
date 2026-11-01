package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountUpdatePasswordSettings
// account.updatePasswordSettings#a59b102f password:InputCheckPasswordSRP new_settings:account.PasswordInputSettings = Bool;
// https://core.telegram.org/method/account.updatePasswordSettings
func (c *TwoFaCore) AccountUpdatePasswordSettings(in *mtproto.TLAccountUpdatePasswordSettings) (*mtproto.Bool, error) {
	var (
		proof    *mtproto.InputCheckPasswordSRP
		settings *mtproto.Account_PasswordInputSettings
	)
	if in != nil {
		proof = in.GetPassword()
		settings = in.GetNewSettings()
	}
	n, err := password.Update(c.ctx, c.userID(), c.authKey(), proof, settings)
	if err != nil {
		c.Logger.Errorf("account.updatePasswordSettings - error: %v", err)
		return nil, err
	}
	if n > 0 {
		err = mtproto.NewErrEmailUnconfirmedX(n)
		c.Logger.Errorf("account.updatePasswordSettings - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
