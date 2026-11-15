package accountrest

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Passkeys are stored in process. There is no WebAuthn verifier, so
// auth.finishPasskeyLogin never returns auth.authorization.
// https://core.telegram.org/api/passkeys
// https://core.telegram.org/method/account.getPasskeys
// https://core.telegram.org/method/account.initPasskeyRegistration
// https://core.telegram.org/method/account.registerPasskey
// https://core.telegram.org/method/account.deletePasskey
// https://core.telegram.org/method/auth.initPasskeyLogin
// https://core.telegram.org/method/auth.finishPasskeyLogin

const passkeyRPID = "localhost"

type passkeyRec struct {
	id     string
	userID int64
	name   string
	date   int32
}

type passkeyStore struct {
	mu         sync.Mutex
	byUser     map[int64][]*passkeyRec
	byID       map[string]int64
	challenges map[int64]string
}

var passkeys = newPasskeyStore()

func newPasskeyStore() *passkeyStore {
	return &passkeyStore{
		byUser:     map[int64][]*passkeyRec{},
		byID:       map[string]int64{},
		challenges: map[int64]string{},
	}
}

func resetPasskeys() {
	passkeys.mu.Lock()
	passkeys.byUser = map[int64][]*passkeyRec{}
	passkeys.byID = map[string]int64{}
	passkeys.challenges = map[int64]string{}
	passkeys.mu.Unlock()
}

// ListPasskeys returns account.passkeys. An account with none gets an empty vector.
func ListPasskeys(userID int64) (*mtproto.Account_Passkeys, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	passkeys.mu.Lock()
	defer passkeys.mu.Unlock()
	rows := passkeys.byUser[userID]
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
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	challenge, err := newChallenge()
	if err != nil {
		return nil, err
	}
	passkeys.mu.Lock()
	passkeys.challenges[userID] = challenge
	passkeys.mu.Unlock()
	userHandle := base64.RawURLEncoding.EncodeToString(userIDBytes(userID))
	body := map[string]any{
		"publicKey": map[string]any{
			"rp": map[string]string{"name": "Tellomi", "id": passkeyRPID},
			"user": map[string]string{
				"id":          userHandle,
				"name":        "user",
				"displayName": "user",
			},
			"challenge": challenge,
			"pubKeyCredParams": []map[string]any{
				{"type": "public-key", "alg": -7},
			},
			"timeout":     60000,
			"attestation": "none",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountPasskeyRegistrationOptions(&mtproto.Account_PasskeyRegistrationOptions{
		Options: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(raw)}).To_DataJSON(),
	}).To_Account_PasskeyRegistrationOptions(), nil
}

// RegisterPasskey stores a registration whose challenge matches InitPasskeyRegistration.
// The attestation is not verified. A bad shape is 400 CREDENTIAL_INVALID.
// An origin other than the issued rpId is 400 PASSKEY_ORIGIN_MISMATCH.
func RegisterPasskey(userID int64, cred *mtproto.InputPasskeyCredential) (*mtproto.Passkey, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if cred.GetPredicateName() != mtproto.Predicate_inputPasskeyCredentialPublicKey ||
		cred.GetId() == "" || cred.GetRawId() == "" ||
		cred.GetResponse().GetPredicateName() != mtproto.Predicate_inputPasskeyResponseRegister ||
		len(cred.GetResponse().GetAttestationData()) == 0 ||
		cred.GetResponse().GetClientData().GetData() == "" {
		return nil, mtproto.ErrCredentialInvalid
	}
	var client struct {
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal([]byte(cred.GetResponse().GetClientData().GetData()), &client); err != nil || client.Challenge == "" {
		return nil, mtproto.ErrCredentialInvalid
	}
	if !originOK(client.Origin) {
		return nil, mtproto.ErrPasskeyOriginMismatch
	}
	passkeys.mu.Lock()
	defer passkeys.mu.Unlock()
	if passkeys.challenges[userID] == "" || passkeys.challenges[userID] != client.Challenge {
		return nil, mtproto.ErrCredentialInvalid
	}
	if owner, ok := passkeys.byID[cred.GetId()]; ok && owner != userID {
		return nil, mtproto.ErrCredentialInvalid
	}
	delete(passkeys.challenges, userID)
	row := &passkeyRec{
		id:     cred.GetId(),
		userID: userID,
		name:   "Passkey",
		date:   int32(time.Now().Unix()),
	}
	list := passkeys.byUser[userID]
	replaced := false
	for i, old := range list {
		if old.id == row.id {
			list[i] = row
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, row)
	}
	passkeys.byUser[userID] = list
	passkeys.byID[row.id] = userID
	return passkeyTL(row), nil
}

// DeletePasskey removes one passkey. A missing id returns boolFalse.
func DeletePasskey(userID int64, id string) (*mtproto.Bool, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if id == "" {
		return mtproto.BoolFalse, nil
	}
	passkeys.mu.Lock()
	defer passkeys.mu.Unlock()
	list := passkeys.byUser[userID]
	next := list[:0]
	found := false
	for _, row := range list {
		if row.id == id {
			found = true
			delete(passkeys.byID, id)
			continue
		}
		next = append(next, row)
	}
	if !found {
		return mtproto.BoolFalse, nil
	}
	if len(next) == 0 {
		delete(passkeys.byUser, userID)
	} else {
		passkeys.byUser[userID] = next
	}
	return mtproto.BoolTrue, nil
}

// InitPasskeyLogin returns PublicKeyCredentialRequestOptions.
// api_id <= 0 or an empty api_hash is 400 API_ID_INVALID.
func InitPasskeyLogin(apiID int32, apiHash string) (*mtproto.Auth_PasskeyLoginOptions, error) {
	if apiID <= 0 || strings.TrimSpace(apiHash) == "" {
		return nil, mtproto.ErrApiIdInvalid
	}
	challenge, err := newChallenge()
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"publicKey": map[string]any{
			"challenge":        challenge,
			"timeout":          60000,
			"rpId":             passkeyRPID,
			"userVerification": "preferred",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLAuthPasskeyLoginOptions(&mtproto.Auth_PasskeyLoginOptions{
		Options: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(raw)}).To_DataJSON(),
	}).To_Auth_PasskeyLoginOptions(), nil
}

// FinishPasskeyLogin never returns auth.authorization.
// A malformed assertion is 400 CREDENTIAL_INVALID.
// An unknown id is 400 PASSKEY_CREDENTIAL_NOT_FOUND.
// A stored id still returns 400 CREDENTIAL_INVALID because the signature is not checked.
func FinishPasskeyLogin(cred *mtproto.InputPasskeyCredential) (*mtproto.Auth_Authorization, error) {
	if cred.GetPredicateName() != mtproto.Predicate_inputPasskeyCredentialPublicKey ||
		cred.GetId() == "" || cred.GetRawId() == "" ||
		cred.GetResponse().GetPredicateName() != mtproto.Predicate_inputPasskeyResponseLogin ||
		cred.GetResponse().GetClientData().GetData() == "" ||
		len(cred.GetResponse().GetAuthenticatorData()) == 0 ||
		len(cred.GetResponse().GetSignature()) == 0 ||
		cred.GetResponse().GetUserHandle() == "" {
		return nil, mtproto.ErrCredentialInvalid
	}
	passkeys.mu.Lock()
	_, known := passkeys.byID[cred.GetId()]
	passkeys.mu.Unlock()
	if !known {
		return nil, mtproto.ErrPasskeyCredentialNotFound
	}
	return nil, mtproto.ErrCredentialInvalid
}

func passkeyTL(row *passkeyRec) *mtproto.Passkey {
	return mtproto.MakeTLPasskey(&mtproto.Passkey{
		Id:   row.id,
		Name: row.name,
		Date: row.date,
	}).To_Passkey()
}

func newChallenge() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func userIDBytes(id int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(id))
	return buf[:]
}

func originOK(origin string) bool {
	if origin == "" || strings.HasPrefix(origin, "android:apk-key-hash:") {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == passkeyRPID || strings.HasSuffix(host, "."+passkeyRPID)
}
