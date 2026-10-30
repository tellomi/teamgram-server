package core

import (
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

// AuthFinishPasskeyLogin
// auth.finishPasskeyLogin#9857ad07 flags:# credential:InputPasskeyCredential from_dc_id:flags.0?int from_auth_key_id:flags.0?long = auth.Authorization;
// https://core.telegram.org/method/auth.finishPasskeyLogin
// https://core.telegram.org/api/passkeys
//
// Passkeys are not stored here, and this method never returns auth.authorization.
// A missing credential is the documented 400 CREDENTIAL_INVALID.
// Any other credential is 400 PASSKEY_CREDENTIAL_NOT_FOUND: the server has no
// matching passkey. auth.initPasskeyLogin stays unimplemented so clients are
// not handed WebAuthn options that cannot be finished.
func (c *PasskeyCore) AuthFinishPasskeyLogin(in *mtproto.TLAuthFinishPasskeyLogin) (*mtproto.Auth_Authorization, error) {
	if !passkeyCredentialOK(in) {
		c.Logger.Errorf("auth.finishPasskeyLogin - error: CREDENTIAL_INVALID")
		return nil, status.Error(mtproto.ErrBadRequest, "CREDENTIAL_INVALID")
	}
	c.Logger.Errorf("auth.finishPasskeyLogin - error: PASSKEY_CREDENTIAL_NOT_FOUND")
	return nil, status.Error(mtproto.ErrBadRequest, "PASSKEY_CREDENTIAL_NOT_FOUND")
}

func passkeyCredentialOK(in *mtproto.TLAuthFinishPasskeyLogin) bool {
	if in == nil || in.GetCredential() == nil {
		return false
	}
	cred := in.GetCredential()
	if cred.GetPredicateName() != mtproto.Predicate_inputPasskeyCredentialPublicKey {
		return false
	}
	if cred.GetId() == "" || cred.GetRawId() == "" || cred.GetResponse() == nil {
		return false
	}
	return true
}
