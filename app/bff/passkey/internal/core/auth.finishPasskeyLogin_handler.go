package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AuthFinishPasskeyLogin
// auth.finishPasskeyLogin#9857ad07 flags:# credential:InputPasskeyCredential from_dc_id:flags.0?int from_auth_key_id:flags.0?long = auth.Authorization;
// https://core.telegram.org/method/auth.finishPasskeyLogin
// https://core.telegram.org/api/passkeys
//
// This server does not verify WebAuthn signatures, so the method never returns
// auth.authorization. A missing credential is 400 CREDENTIAL_INVALID. An unknown
// id is 400 PASSKEY_CREDENTIAL_NOT_FOUND. A stored id is 400 CREDENTIAL_INVALID.
func (c *PasskeyCore) AuthFinishPasskeyLogin(in *mtproto.TLAuthFinishPasskeyLogin) (*mtproto.Auth_Authorization, error) {
	if in == nil {
		return nil, mtproto.ErrCredentialInvalid
	}
	return accountrest.FinishPasskeyLogin(in.GetCredential())
}
