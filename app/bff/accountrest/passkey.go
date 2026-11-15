package accountrest

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passkey/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Passkeys are stored in MySQL by app/bff/passkey/persist.
// auth.finishPasskeyLogin verifies the assertion there, then the passkey
// handler binds the auth key through app/bff/passkey/login.
// https://core.telegram.org/api/passkeys

func resetPasskeys() {
	persist.ResetForTest()
}

// ListPasskeys returns account.passkeys. An account with none gets an empty vector.
func ListPasskeys(userID int64) (*mtproto.Account_Passkeys, error) {
	rows, err := persist.List(userID)
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.Passkey, 0, len(rows))
	for _, row := range rows {
		out = append(out, passkeyTL(row))
	}
	return mtproto.MakeTLAccountPasskeys(&mtproto.Account_Passkeys{
		Passkeys: out,
	}).To_Account_Passkeys(), nil
}

// InitPasskeyRegistration returns PublicKeyCredentialCreationOptions for this account.
func InitPasskeyRegistration(userID int64) (*mtproto.Account_PasskeyRegistrationOptions, error) {
	raw, err := persist.InitRegistration(userID)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountPasskeyRegistrationOptions(&mtproto.Account_PasskeyRegistrationOptions{
		Options: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: raw}).To_DataJSON(),
	}).To_Account_PasskeyRegistrationOptions(), nil
}

// RegisterPasskey stores a verified registration.
func RegisterPasskey(userID int64, cred *mtproto.InputPasskeyCredential) (*mtproto.Passkey, error) {
	row, err := persist.Register(userID, cred)
	if err != nil {
		return nil, err
	}
	return passkeyTL(row), nil
}

// DeletePasskey removes one passkey. A missing id returns boolFalse.
func DeletePasskey(userID int64, id string) (*mtproto.Bool, error) {
	ok, err := persist.Delete(userID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return mtproto.BoolFalse, nil
	}
	return mtproto.BoolTrue, nil
}

// InitPasskeyLogin returns PublicKeyCredentialRequestOptions.
// api_id <= 0 or an empty api_hash is 400 API_ID_INVALID.
func InitPasskeyLogin(apiID int32, apiHash string) (*mtproto.Auth_PasskeyLoginOptions, error) {
	if apiID <= 0 || strings.TrimSpace(apiHash) == "" {
		return nil, mtproto.ErrApiIdInvalid
	}
	raw, err := persist.InitLogin()
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAuthPasskeyLoginOptions(&mtproto.Auth_PasskeyLoginOptions{
		Options: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: raw}).To_DataJSON(),
	}).To_Auth_PasskeyLoginOptions(), nil
}

// VerifyPasskeyLogin checks the assertion and returns the account id.
// The handler binds the auth key and returns auth.authorization.
func VerifyPasskeyLogin(cred *mtproto.InputPasskeyCredential) (int64, error) {
	return persist.VerifyLogin(cred)
}

func passkeyTL(row persist.Saved) *mtproto.Passkey {
	key := &mtproto.Passkey{
		Id:   row.ID,
		Name: row.Name,
		Date: row.Date,
	}
	if row.LastUsed > 0 {
		key.LastUsageDate = wrapperspb.Int32(row.LastUsed)
	}
	return mtproto.MakeTLPasskey(key).To_Passkey()
}
