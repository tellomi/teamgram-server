package core

import (
	"github.com/teamgram/proto/mtproto"
)

// AccountGetPassword
// account.getPassword#548a30f5 = account.Password;
// https://core.telegram.org/method/account.getPassword
// https://core.telegram.org/constructor/account.password
// https://core.telegram.org/api/srp
func (c *TwoFaCore) AccountGetPassword(_ *mtproto.TLAccountGetPassword) (*mtproto.Account_Password, error) {
	return c.accountPassword(), nil
}
