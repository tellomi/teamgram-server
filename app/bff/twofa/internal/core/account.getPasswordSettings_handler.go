package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountGetPasswordSettings
// account.getPasswordSettings#9cd4eaf9 password:InputCheckPasswordSRP = account.PasswordSettings;
// https://core.telegram.org/method/account.getPasswordSettings
// https://core.telegram.org/constructor/account.passwordSettings
func (c *TwoFaCore) AccountGetPasswordSettings(in *mtproto.TLAccountGetPasswordSettings) (*mtproto.Account_PasswordSettings, error) {
	var password *mtproto.InputCheckPasswordSRP
	if in != nil {
		password = in.GetPassword()
	}
	// This method's documented failure for a bad proof is PASSWORD_HASH_INVALID.
	if err := c.checkEmptyPassword(password); err != nil {
		c.Logger.Errorf("account.getPasswordSettings - error: %v", err)
		return nil, mtproto.ErrPasswordHashInvalid
	}
	return emptyPasswordSettings(), nil
}
