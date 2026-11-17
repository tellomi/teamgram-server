package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
	passkeylogin "github.com/teamgram/teamgram-server/app/bff/passkey/login"
	"github.com/teamgram/teamgram-server/app/bff/passkey/persist"
)

// AuthFinishPasskeyLogin
// auth.finishPasskeyLogin#9857ad07 flags:# credential:InputPasskeyCredential from_dc_id:flags.0?int from_auth_key_id:flags.0?long = auth.Authorization;
// https://core.telegram.org/method/auth.finishPasskeyLogin
// https://core.telegram.org/api/passkeys
//
// A verified assertion binds this auth key and returns auth.authorization.
// A cloud password returns 401 SESSION_PASSWORD_NEEDED after the bind.
// A malformed assertion is 400 CREDENTIAL_INVALID. An unknown id is
// 400 PASSKEY_CREDENTIAL_NOT_FOUND. A bad origin is 400 PASSKEY_ORIGIN_MISMATCH.
func (c *PasskeyCore) AuthFinishPasskeyLogin(in *mtproto.TLAuthFinishPasskeyLogin) (*mtproto.Auth_Authorization, error) {
	if in == nil {
		return nil, mtproto.ErrCredentialInvalid
	}
	if dc := in.GetFromDcId(); dc != nil && dc.GetValue() != 0 && dc.GetValue() != persist.DC() {
		return nil, mtproto.ErrDcIdInvalid
	}
	userID, err := accountrest.VerifyPasskeyLogin(c.permAuthKey(), in.GetCredential())
	if err != nil {
		return nil, err
	}
	return passkeylogin.Authorize(c.ctx, c.MD, userID)
}
