package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountGetPasskeys
// account.getPasskeys#ea1f0c52 = account.Passkeys;
// https://core.telegram.org/method/account.getPasskeys
func (c *PasskeyCore) AccountGetPasskeys(in *mtproto.TLAccountGetPasskeys) (*mtproto.Account_Passkeys, error) {
	return accountrest.ListPasskeys(c.userID())
}

func (c *PasskeyCore) userID() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.GetUserId()
}
