package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestPasskeyLogin registers a passkey, logs in on a new auth key, rejects a
// bad signature and an unknown credential, then logs in again. When
// TELLOMI_E2E_RESTART is set the teamgram process is restarted first, so the
// second login is read from MySQL. The phone is in the 999661 CI range.
func TestPasskeyLogin(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(4 * time.Minute)
	defer cancel()
	owner, self := dialPhone(t, ctx, cfg, "9996610776", "Passkey")

	reg, err := owner.Invoke(ctx, &mtproto.TLAccountInitPasskeyRegistration{
		Constructor: mtproto.CRC32_account_initPasskeyRegistration,
	})
	if err != nil {
		t.Fatal(err)
	}
	opts := passkeyOptions(t, reg)
	handle, err := base64.RawURLEncoding.DecodeString(opts.userID)
	if err != nil || len(handle) == 0 {
		t.Fatalf("user handle %q", opts.userID)
	}
	key := newE2ESoftKey(t, opts.rpID, handle)
	origin := "https://" + opts.rpID
	saved, err := owner.Invoke(ctx, &mtproto.TLAccountRegisterPasskey{
		Constructor: mtproto.CRC32_account_registerPasskey,
		Credential:  key.register(t, opts.challenge, origin),
	})
	if err != nil {
		t.Fatal(err)
	}
	if passkeyOf(t, saved).GetId() != key.idString() {
		t.Fatalf("registered %#v", saved)
	}

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	logged := passkeySignIn(t, ctx, cfg, fresh, key, origin)
	if logged.ID != self.ID {
		t.Fatalf("passkey user %d, phone user %d", logged.ID, self.ID)
	}
	listed, err := fresh.Invoke(ctx, &mtproto.TLAccountGetPasskeys{
		Constructor: mtproto.CRC32_account_getPasskeys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passkeyListHas(t, listed, key.idString()) {
		t.Fatalf("getPasskeys %#v", listed)
	}

	bad := passkeyAssertion(t, ctx, cfg, key, origin)
	bad.GetResponse().Signature[0] ^= 0xff
	_, err = fresh.Invoke(ctx, &mtproto.TLAuthFinishPasskeyLogin{
		Constructor: mtproto.CRC32_auth_finishPasskeyLogin,
		Credential:  bad,
	})
	assertRPC(t, err, 400, "CREDENTIAL_INVALID")

	unknown := passkeyAssertion(t, ctx, cfg, key, origin)
	unknown.Id = base64.RawURLEncoding.EncodeToString([]byte("not-this-passkey!!"))
	unknown.RawId = unknown.Id
	_, err = fresh.Invoke(ctx, &mtproto.TLAuthFinishPasskeyLogin{
		Constructor: mtproto.CRC32_auth_finishPasskeyLogin,
		Credential:  unknown,
	})
	assertRPC(t, err, 400, "PASSKEY_CREDENTIAL_NOT_FOUND")

	if os.Getenv("TELLOMI_E2E_RESTART") != "" {
		if err = restartTeamgram(ctx); err != nil {
			t.Fatal(err)
		}
		_ = waitTCPDown(ctx, cfg.Addr)
		if err = waitTCP(ctx, cfg.Addr); err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(8 * time.Second):
		}
	}
	again, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	logged = passkeySignIn(t, ctx, cfg, again, key, origin)
	if logged.ID != self.ID {
		t.Fatalf("login after reread user %d", logged.ID)
	}
}

func passkeySignIn(t *testing.T, ctx context.Context, cfg Config, c *Client, key *e2eSoftKey, origin string) Self {
	t.Helper()
	cred := passkeyAssertion(t, ctx, cfg, key, origin)
	obj, err := c.Invoke(ctx, &mtproto.TLAuthFinishPasskeyLogin{
		Constructor: mtproto.CRC32_auth_finishPasskeyLogin,
		Credential:  cred,
	})
	if err != nil {
		t.Fatal(err)
	}
	authz, ok := obj.(*mtproto.TLAuthAuthorization)
	if !ok {
		t.Fatalf("finishPasskeyLogin %T", obj)
	}
	self, err := selfFrom(authz.GetUser())
	if err != nil {
		t.Fatal(err)
	}
	return self
}

func passkeyAssertion(t *testing.T, ctx context.Context, cfg Config, key *e2eSoftKey, origin string) *mtproto.InputPasskeyCredential {
	t.Helper()
	anon, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	defer anon.Close()
	obj, err := anon.Invoke(ctx, &mtproto.TLAuthInitPasskeyLogin{
		Constructor: mtproto.CRC32_auth_initPasskeyLogin,
		ApiId:       cfg.APIID,
		ApiHash:     cfg.APIHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	opts := passkeyOptions(t, obj)
	return key.login(t, opts.challenge, origin)
}

type passkeyOpts struct {
	challenge string
	rpID      string
	userID    string
}

func passkeyOptions(t *testing.T, obj mtproto.TLObject) passkeyOpts {
	t.Helper()
	var data string
	switch v := obj.(type) {
	case *mtproto.TLAccountPasskeyRegistrationOptions:
		data = v.GetOptions().GetData()
	case *mtproto.TLAuthPasskeyLoginOptions:
		data = v.GetOptions().GetData()
	default:
		t.Fatalf("options %T", obj)
	}
	var body struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RpID      string `json:"rpId"`
			Rp        struct {
				ID string `json:"id"`
			} `json:"rp"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		t.Fatal(err)
	}
	rp := body.PublicKey.Rp.ID
	if rp == "" {
		rp = body.PublicKey.RpID
	}
	if body.PublicKey.Challenge == "" || rp == "" {
		t.Fatal(data)
	}
	return passkeyOpts{challenge: body.PublicKey.Challenge, rpID: rp, userID: body.PublicKey.User.ID}
}

func passkeyOf(t *testing.T, obj mtproto.TLObject) *mtproto.Passkey {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLPasskey:
		return v.To_Passkey()
	case *mtproto.Passkey:
		return v
	default:
		t.Fatalf("passkey %T", obj)
		return nil
	}
}

func passkeyListHas(t *testing.T, obj mtproto.TLObject, id string) bool {
	t.Helper()
	var rows []*mtproto.Passkey
	switch v := obj.(type) {
	case *mtproto.TLAccountPasskeys:
		rows = v.GetPasskeys()
	case *mtproto.Account_Passkeys:
		rows = v.GetPasskeys()
	default:
		t.Fatalf("passkeys %T", obj)
	}
	for _, row := range rows {
		if row.GetId() == id {
			return true
		}
	}
	return false
}

type e2eSoftKey struct {
	priv    *ecdsa.PrivateKey
	id      []byte
	rpID    string
	handle  []byte
	counter uint32
}

func newE2ESoftKey(t *testing.T, rpID string, handle []byte) *e2eSoftKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return &e2eSoftKey{priv: priv, id: id, rpID: rpID, handle: handle}
}

func (k *e2eSoftKey) idString() string { return base64.RawURLEncoding.EncodeToString(k.id) }

func (k *e2eSoftKey) register(t *testing.T, challenge, origin string) *mtproto.InputPasskeyCredential {
	t.Helper()
	client := k.clientData(t, "webauthn.create", challenge, origin)
	auth := k.authData(0x45, k.counter)
	id := k.idString()
	return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
		Id:    id,
		RawId: id,
		Response: mtproto.MakeTLInputPasskeyResponseRegister(&mtproto.InputPasskeyResponse{
			ClientData:      mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(client)}).To_DataJSON(),
			AttestationData: e2eAttestationNone(auth),
		}).To_InputPasskeyResponse(),
	}).To_InputPasskeyCredential()
}

func (k *e2eSoftKey) login(t *testing.T, challenge, origin string) *mtproto.InputPasskeyCredential {
	t.Helper()
	k.counter++
	client := k.clientData(t, "webauthn.get", challenge, origin)
	auth := k.authData(0x05, k.counter)
	id := k.idString()
	return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
		Id:    id,
		RawId: id,
		Response: mtproto.MakeTLInputPasskeyResponseLogin(&mtproto.InputPasskeyResponse{
			ClientData:        mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(client)}).To_DataJSON(),
			AuthenticatorData: auth,
			Signature:         k.sign(t, auth, client),
			UserHandle:        string(k.handle),
		}).To_InputPasskeyResponse(),
	}).To_InputPasskeyCredential()
}

func (k *e2eSoftKey) clientData(t *testing.T, ceremony, challenge, origin string) []byte {
	t.Helper()
	raw, err := json.Marshal(struct {
		Type        string `json:"type"`
		Challenge   string `json:"challenge"`
		Origin      string `json:"origin"`
		CrossOrigin bool   `json:"crossOrigin"`
	}{ceremony, challenge, origin, false})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (k *e2eSoftKey) authData(flags byte, counter uint32) []byte {
	rp := sha256.Sum256([]byte(k.rpID))
	out := append(append([]byte{}, rp[:]...), flags)
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], counter)
	out = append(out, count[:]...)
	if flags&0x40 == 0 {
		return out
	}
	out = append(out, make([]byte, 16)...)
	var n [2]byte
	binary.BigEndian.PutUint16(n[:], uint16(len(k.id)))
	out = append(out, n[:]...)
	out = append(out, k.id...)
	x := k.priv.PublicKey.X.FillBytes(make([]byte, 32))
	y := k.priv.PublicKey.Y.FillBytes(make([]byte, 32))
	cose := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	cose = append(cose, x...)
	cose = append(cose, 0x22, 0x58, 0x20)
	cose = append(cose, y...)
	return append(out, cose...)
}

func (k *e2eSoftKey) sign(t *testing.T, auth, client []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(client)
	msg := sha256.Sum256(append(append([]byte{}, auth...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, msg[:])
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func e2eAttestationNone(auth []byte) []byte {
	out := []byte{0xa3, 0x63, 'f', 'm', 't', 0x64, 'n', 'o', 'n', 'e', 0x67, 'a', 't', 't', 'S', 't', 'm', 't', 0xa0, 0x68, 'a', 'u', 't', 'h', 'D', 'a', 't', 'a', 0x58, byte(len(auth))}
	return append(out, auth...)
}
