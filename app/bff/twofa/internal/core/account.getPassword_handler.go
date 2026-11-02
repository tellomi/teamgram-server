package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// AccountGetPassword
// account.getPassword#548a30f5 = account.Password;
// https://core.telegram.org/method/account.getPassword
// https://core.telegram.org/constructor/account.password
// https://core.telegram.org/api/srp
func (c *TwoFaCore) AccountGetPassword(_ *mtproto.TLAccountGetPassword) (*mtproto.Account_Password, error) {
	userID := c.userID()
	// auth.logOut on this permanent key leaves the session user id at 0.
	// The password step is recorded on the auth key before that id is restored.
	if userID == 0 {
		id, err := password.PendingUser(c.ctx, c.authKey())
		if err != nil {
			c.Logger.Errorf("account.getPassword - pending: %v", err)
			return nil, err
		}
		userID = id
	}
	return password.AccountPassword(c.ctx, userID, c.authKey())
}
