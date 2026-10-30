package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountUpdatePasswordSettings
// account.updatePasswordSettings#a59b102f password:InputCheckPasswordSRP new_settings:account.PasswordInputSettings = Bool;
// https://core.telegram.org/method/account.updatePasswordSettings
func (c *TwoFaCore) AccountUpdatePasswordSettings(in *mtproto.TLAccountUpdatePasswordSettings) (*mtproto.Bool, error) {
	var (
		password *mtproto.InputCheckPasswordSRP
		settings *mtproto.Account_PasswordInputSettings
	)
	if in != nil {
		password = in.GetPassword()
		settings = in.GetNewSettings()
	}
	if err := c.checkEmptyPassword(password); err != nil {
		c.Logger.Errorf("account.updatePasswordSettings - error: %v", err)
		return nil, err
	}
	if err := validateNewSettings(settings); err != nil {
		c.Logger.Errorf("account.updatePasswordSettings - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
