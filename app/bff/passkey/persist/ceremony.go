package persist

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/teamgram/proto/mtproto"
)

var (
	errNotFound = errors.New("passkey credential not found")
	errInvalid  = errors.New("passkey credential invalid")
	errOrigin   = errors.New("passkey origin mismatch")
	errNoStore  = errors.New("passkey store is not open")
)

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errNotFound) {
		return mtproto.ErrPasskeyCredentialNotFound
	}
	if errors.Is(err, errOrigin) {
		return mtproto.ErrPasskeyOriginMismatch
	}
	if errors.Is(err, errNoStore) {
		return mtproto.ErrInternalServerError
	}
	var protoErr *protocol.Error
	if errors.As(err, &protoErr) {
		text := strings.ToLower(protoErr.Details + " " + protoErr.DevInfo)
		if strings.Contains(text, "origin") {
			return mtproto.ErrPasskeyOriginMismatch
		}
		return mtproto.ErrCredentialInvalid
	}
	if errors.Is(err, errInvalid) {
		return mtproto.ErrCredentialInvalid
	}
	return mtproto.ErrInternalServerError
}

func mustStore() (*Service, error) {
	s := Current()
	if s == nil || s.db == nil {
		return nil, errNoStore
	}
	return s, nil
}

type sessionBlob struct {
	Data    webauthn.SessionData
	raw     string
	expires int32
}

func encodeSession(data *webauthn.SessionData) (*sessionBlob, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	exp := int32(time.Now().Add(time.Minute).Unix())
	if !data.Expires.IsZero() {
		exp = int32(data.Expires.Unix())
	}
	return &sessionBlob{Data: *data, raw: string(raw), expires: exp}, nil
}

func decodeSession(raw string) (*sessionBlob, error) {
	var data webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, errInvalid
	}
	return &sessionBlob{Data: data, raw: raw, expires: int32(data.Expires.Unix())}, nil
}

// InitRegistration returns the PublicKeyCredentialCreationOptions JSON.
func InitRegistration(userID int64) (string, error) {
	if userID == 0 {
		return "", mtproto.ErrAuthKeyUnregistered
	}
	s, err := mustStore()
	if err != nil {
		return "", mapErr(err)
	}
	wa, err := newWebAuthn("")
	if err != nil {
		return "", mapErr(err)
	}
	rawIDs, err := s.rawIDs(userID)
	if err != nil {
		return "", mapErr(err)
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(rawIDs))
	for _, raw := range rawIDs {
		exclude = append(exclude, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: raw,
		})
	}
	user := waUser{handle: []byte(userHandle(userID)), name: "user"}
	creation, session, err := wa.BeginRegistration(user, webauthn.WithExclusions(exclude))
	if err != nil {
		return "", mapErr(err)
	}
	blob, err := encodeSession(session)
	if err != nil {
		return "", mapErr(err)
	}
	if err = s.saveChallenge(userID, "register", blob); err != nil {
		return "", mapErr(err)
	}
	raw, err := json.Marshal(creation)
	if err != nil {
		return "", mapErr(err)
	}
	return string(raw), nil
}

// Register checks the attestation and stores the credential id, public key, and sign count.
func Register(userID int64, cred *mtproto.InputPasskeyCredential) (Saved, error) {
	if userID == 0 {
		return Saved{}, mtproto.ErrAuthKeyUnregistered
	}
	parsed, client, err := registrationBody(cred)
	if err != nil {
		return Saved{}, mapErr(err)
	}
	if !originAllowed(client.Origin) {
		return Saved{}, mtproto.ErrPasskeyOriginMismatch
	}
	s, err := mustStore()
	if err != nil {
		return Saved{}, mapErr(err)
	}
	blob, err := s.takeChallenge(client.Challenge, "register", userID, true)
	if err != nil {
		return Saved{}, mapErr(err)
	}
	wa, err := newWebAuthn(client.Origin)
	if err != nil {
		return Saved{}, mapErr(err)
	}
	user := waUser{handle: []byte(userHandle(userID)), name: "user"}
	credential, err := wa.CreateCredential(user, blob.Data, parsed)
	if err != nil {
		return Saved{}, mapErr(err)
	}
	if credential.Authenticator.CloneWarning {
		return Saved{}, mtproto.ErrCredentialInvalid
	}
	declared, decErr := base64.RawURLEncoding.DecodeString(cred.GetId())
	if decErr != nil || !bytes.Equal(declared, credential.ID) {
		return Saved{}, mtproto.ErrCredentialInvalid
	}
	now := int32(time.Now().Unix())
	row := credentialRow{
		ID:              cred.GetId(),
		UserID:          userID,
		RawID:           append([]byte(nil), credential.ID...),
		PublicKey:       append([]byte(nil), credential.PublicKey...),
		SignCount:       credential.Authenticator.SignCount,
		AAGUID:          padAAGUID(credential.Authenticator.AAGUID),
		AttestationType: credential.AttestationType,
		UserHandle:      userHandle(userID),
		UserPresent:     credential.Flags.UserPresent,
		UserVerified:    credential.Flags.UserVerified,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		Name:            "Passkey",
		CreatedAt:       now,
	}
	if err = s.insert(row); err != nil {
		return Saved{}, mapErr(err)
	}
	return Saved{ID: row.ID, Name: row.Name, Date: row.CreatedAt}, nil
}

// List returns the account's passkeys.
func List(userID int64) ([]Saved, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	s, err := mustStore()
	if err != nil {
		return nil, mapErr(err)
	}
	rows, err := s.list(userID)
	if err != nil {
		return nil, mapErr(err)
	}
	return rows, nil
}

// Delete removes one passkey. A missing id is false.
func Delete(userID int64, id string) (bool, error) {
	if userID == 0 {
		return false, mtproto.ErrAuthKeyUnregistered
	}
	if id == "" {
		return false, nil
	}
	s, err := mustStore()
	if err != nil {
		return false, mapErr(err)
	}
	raw, decErr := base64.RawURLEncoding.DecodeString(id)
	if decErr != nil || len(raw) == 0 {
		return false, nil
	}
	ok, err := s.delete(userID, raw)
	if err != nil {
		return false, mapErr(err)
	}
	return ok, nil
}

// InitLogin returns the PublicKeyCredentialRequestOptions JSON.
func InitLogin() (string, error) {
	s, err := mustStore()
	if err != nil {
		return "", mapErr(err)
	}
	wa, err := newWebAuthn("")
	if err != nil {
		return "", mapErr(err)
	}
	assertion, session, err := wa.BeginDiscoverableLogin()
	if err != nil {
		return "", mapErr(err)
	}
	blob, err := encodeSession(session)
	if err != nil {
		return "", mapErr(err)
	}
	if err = s.saveChallenge(0, "login", blob); err != nil {
		return "", mapErr(err)
	}
	raw, err := json.Marshal(assertion)
	if err != nil {
		return "", mapErr(err)
	}
	return string(raw), nil
}

// VerifyLogin checks the assertion and returns the user id.
// The sign count is stored only when it moved forward.
func VerifyLogin(cred *mtproto.InputPasskeyCredential) (int64, error) {
	parsed, client, err := loginBody(cred)
	if err != nil {
		return 0, mapErr(err)
	}
	if !originAllowed(client.Origin) {
		return 0, mtproto.ErrPasskeyOriginMismatch
	}
	s, err := mustStore()
	if err != nil {
		return 0, mapErr(err)
	}
	row, err := s.byRaw(parsed.RawID)
	if err != nil {
		return 0, mapErr(err)
	}
	if !bytes.Equal(row.RawID, parsed.RawID) {
		return 0, mtproto.ErrCredentialInvalid
	}
	blob, err := s.takeChallenge(client.Challenge, "login", 0, false)
	if err != nil {
		return 0, mapErr(err)
	}
	wa, err := newWebAuthn(client.Origin)
	if err != nil {
		return 0, mapErr(err)
	}
	stored := webauthn.Credential{
		ID:              append([]byte(nil), row.RawID...),
		PublicKey:       append([]byte(nil), row.PublicKey...),
		AttestationType: row.AttestationType,
		Flags: webauthn.CredentialFlags{
			UserPresent:    row.UserPresent,
			UserVerified:   row.UserVerified,
			BackupEligible: row.BackupEligible,
			BackupState:    row.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    append([]byte(nil), row.AAGUID...),
			SignCount: row.SignCount,
		},
	}
	user := waUser{handle: []byte(row.UserHandle), name: "user", creds: []webauthn.Credential{stored}}
	_, updated, err := wa.ValidatePasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		if !bytes.Equal(rawID, row.RawID) || !bytes.Equal(userHandle, []byte(row.UserHandle)) {
			return nil, errInvalid
		}
		return user, nil
	}, blob.Data, parsed)
	if err != nil {
		return 0, mapErr(err)
	}
	if updated.Authenticator.CloneWarning {
		return 0, mtproto.ErrCredentialInvalid
	}
	row.UserPresent = updated.Flags.UserPresent
	row.UserVerified = updated.Flags.UserVerified
	row.BackupEligible = updated.Flags.BackupEligible
	row.BackupState = updated.Flags.BackupState
	used := int32(time.Now().Unix())
	if err = s.saveCounter(row.RawID, row.SignCount, updated.Authenticator.SignCount, row, used); err != nil {
		return 0, mapErr(err)
	}
	return row.UserID, nil
}

type clientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

func registrationBody(cred *mtproto.InputPasskeyCredential) (*protocol.ParsedCredentialCreationData, clientData, error) {
	if cred.GetPredicateName() != mtproto.Predicate_inputPasskeyCredentialPublicKey ||
		cred.GetId() == "" || cred.GetRawId() == "" ||
		cred.GetResponse().GetPredicateName() != mtproto.Predicate_inputPasskeyResponseRegister ||
		len(cred.GetResponse().GetAttestationData()) == 0 ||
		cred.GetResponse().GetClientData().GetData() == "" {
		return nil, clientData{}, errInvalid
	}
	if _, err := sameRawID(cred.GetId(), cred.GetRawId()); err != nil {
		return nil, clientData{}, err
	}
	var client clientData
	if err := json.Unmarshal([]byte(cred.GetResponse().GetClientData().GetData()), &client); err != nil || client.Challenge == "" {
		return nil, clientData{}, errInvalid
	}
	body, err := json.Marshal(map[string]any{
		"id":    cred.GetId(),
		"rawId": cred.GetRawId(),
		"type":  "public-key",
		"response": map[string]string{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString([]byte(cred.GetResponse().GetClientData().GetData())),
			"attestationObject": base64.RawURLEncoding.EncodeToString(cred.GetResponse().GetAttestationData()),
		},
	})
	if err != nil {
		return nil, clientData{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return nil, clientData{}, errInvalid
	}
	return parsed, client, nil
}

func loginBody(cred *mtproto.InputPasskeyCredential) (*protocol.ParsedCredentialAssertionData, clientData, error) {
	if cred.GetPredicateName() != mtproto.Predicate_inputPasskeyCredentialPublicKey ||
		cred.GetId() == "" || cred.GetRawId() == "" ||
		cred.GetResponse().GetPredicateName() != mtproto.Predicate_inputPasskeyResponseLogin ||
		cred.GetResponse().GetClientData().GetData() == "" ||
		len(cred.GetResponse().GetAuthenticatorData()) == 0 ||
		len(cred.GetResponse().GetSignature()) == 0 ||
		cred.GetResponse().GetUserHandle() == "" {
		return nil, clientData{}, errInvalid
	}
	if _, err := sameRawID(cred.GetId(), cred.GetRawId()); err != nil {
		return nil, clientData{}, err
	}
	var client clientData
	if err := json.Unmarshal([]byte(cred.GetResponse().GetClientData().GetData()), &client); err != nil || client.Challenge == "" {
		return nil, clientData{}, errInvalid
	}
	body, err := json.Marshal(map[string]any{
		"id":    cred.GetId(),
		"rawId": cred.GetRawId(),
		"type":  "public-key",
		"response": map[string]string{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString([]byte(cred.GetResponse().GetClientData().GetData())),
			"authenticatorData": base64.RawURLEncoding.EncodeToString(cred.GetResponse().GetAuthenticatorData()),
			"signature":         base64.RawURLEncoding.EncodeToString(cred.GetResponse().GetSignature()),
			"userHandle":        base64.RawURLEncoding.EncodeToString([]byte(cred.GetResponse().GetUserHandle())),
		},
	})
	if err != nil {
		return nil, clientData{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		return nil, clientData{}, errInvalid
	}
	return parsed, client, nil
}

func sameRawID(id, raw string) ([]byte, error) {
	a, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(a) == 0 {
		return nil, errInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || !bytes.Equal(a, b) {
		return nil, errInvalid
	}
	return a, nil
}

func padAAGUID(in []byte) []byte {
	out := make([]byte, 16)
	copy(out, in)
	return out
}
