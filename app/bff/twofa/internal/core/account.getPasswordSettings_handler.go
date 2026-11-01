package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountGetPasswordSettings
// account.getPasswordSettings#9cd4eaf9 password:InputCheckPasswordSRP = account.PasswordSettings;
// https://core.telegram.org/method/account.getPasswordSettings
// https://core.telegram.org/constructor/account.passwordSettings
func (c *TwoFaCore) AccountGetPasswordSettings(in *mtproto.TLAccountGetPasswordSettings) (*mtproto.Account_PasswordSettings, error) {
	var proof *mtproto.InputCheckPasswordSRP
	if in != nil {
		proof = in.GetPassword()
	}
	settings, err := password.Settings(c.ctx, c.userID(), c.authKey(), proof)
	if err != nil {
		c.Logger.Errorf("account.getPasswordSettings - error: %v", err)
		return nil, err
	}
	return settings, nil
}
