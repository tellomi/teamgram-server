package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountDeletePasskey
// account.deletePasskey#f5b5563f id:string = Bool;
// https://core.telegram.org/method/account.deletePasskey
func (c *PasskeyCore) AccountDeletePasskey(in *mtproto.TLAccountDeletePasskey) (*mtproto.Bool, error) {
	id := ""
	if in != nil {
		id = in.GetId()
	}
	return accountrest.DeletePasskey(c.userID(), id)
}
