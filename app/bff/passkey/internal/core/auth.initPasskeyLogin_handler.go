package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AuthInitPasskeyLogin
// auth.initPasskeyLogin#518ad0b7 api_id:int api_hash:string = auth.PasskeyLoginOptions;
// https://core.telegram.org/method/auth.initPasskeyLogin
//
// Android calls this constructor from TL_account.initPasskeyLogin. The layer-232
// method is auth.initPasskeyLogin. There is no account.initPasskeyLogin.
func (c *PasskeyCore) AuthInitPasskeyLogin(in *mtproto.TLAuthInitPasskeyLogin) (*mtproto.Auth_PasskeyLoginOptions, error) {
	if in == nil {
		return nil, mtproto.ErrApiIdInvalid
	}
	return accountrest.InitPasskeyLogin(in.GetApiId(), in.GetApiHash())
}
