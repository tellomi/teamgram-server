package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountRegisterPasskey
// account.registerPasskey#55b41fd6 credential:InputPasskeyCredential = Passkey;
// https://core.telegram.org/method/account.registerPasskey
func (c *PasskeyCore) AccountRegisterPasskey(in *mtproto.TLAccountRegisterPasskey) (*mtproto.Passkey, error) {
	if in == nil {
		return nil, mtproto.ErrCredentialInvalid
	}
	return accountrest.RegisterPasskey(c.userID(), in.GetCredential())
}
