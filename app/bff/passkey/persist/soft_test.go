package persist_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// softKey is a P-256 passkey. Registration uses sign count 0. Each login
// assertion uses the next count so a replay is rejected.
type softKey struct {
	priv    *ecdsa.PrivateKey
	id      []byte
	rpID    string
	handle  []byte
	counter uint32
}

func newSoftKey(t *testing.T, rpID string, handle []byte) *softKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return &softKey{priv: priv, id: id, rpID: rpID, handle: handle}
}

func (k *softKey) idString() string {
	return base64.RawURLEncoding.EncodeToString(k.id)
}

func (k *softKey) register(t *testing.T, challenge, origin string) *mtproto.InputPasskeyCredential {
	t.Helper()
	auth := k.authData(0x45, k.counter)
	return k.credential(t, "webauthn.create", challenge, origin, auth, nil)
}

func (k *softKey) login(t *testing.T, challenge, origin string) *mtproto.InputPasskeyCredential {
	t.Helper()
	k.counter++
	auth := k.authData(0x05, k.counter)
	return k.credential(t, "webauthn.get", challenge, origin, auth, k.sign(t, auth, k.clientData(t, "webauthn.get", challenge, origin)))
}

func (k *softKey) loginAt(t *testing.T, challenge, origin string, counter uint32) *mtproto.InputPasskeyCredential {
	t.Helper()
	auth := k.authData(0x05, counter)
	return k.credential(t, "webauthn.get", challenge, origin, auth, k.sign(t, auth, k.clientData(t, "webauthn.get", challenge, origin)))
}

func (k *softKey) credential(t *testing.T, ceremony, challenge, origin string, auth, sig []byte) *mtproto.InputPasskeyCredential {
	t.Helper()
	client := k.clientData(t, ceremony, challenge, origin)
	id := k.idString()
	if ceremony == "webauthn.create" {
		return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
			Id:    id,
			RawId: id,
			Response: mtproto.MakeTLInputPasskeyResponseRegister(&mtproto.InputPasskeyResponse{
				ClientData:      mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(client)}).To_DataJSON(),
				AttestationData: attestationNone(auth),
			}).To_InputPasskeyResponse(),
		}).To_InputPasskeyCredential()
	}
	return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
		Id:    id,
		RawId: id,
		Response: mtproto.MakeTLInputPasskeyResponseLogin(&mtproto.InputPasskeyResponse{
			ClientData:        mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(client)}).To_DataJSON(),
			AuthenticatorData: auth,
			Signature:         sig,
			UserHandle:        string(k.handle),
		}).To_InputPasskeyResponse(),
	}).To_InputPasskeyCredential()
}

func (k *softKey) clientData(t *testing.T, ceremony, challenge, origin string) []byte {
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

func (k *softKey) authData(flags byte, counter uint32) []byte {
	rp := sha256.Sum256([]byte(k.rpID))
	out := make([]byte, 0, 148)
	out = append(out, rp[:]...)
	out = append(out, flags)
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
	out = append(out, coseP256(k.priv.PublicKey)...)
	return out
}

func (k *softKey) sign(t *testing.T, auth, client []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(client)
	msg := sha256.Sum256(append(append([]byte{}, auth...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, msg[:])
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func coseP256(pub ecdsa.PublicKey) []byte {
	x := pub.X.FillBytes(make([]byte, 32))
	y := pub.Y.FillBytes(make([]byte, 32))
	out := []byte{
		0xa5,
		0x01, 0x02,
		0x03, 0x26,
		0x20, 0x01,
		0x21, 0x58, 0x20,
	}
	out = append(out, x...)
	out = append(out, 0x22, 0x58, 0x20)
	out = append(out, y...)
	return out
}

func attestationNone(auth []byte) []byte {
	out := []byte{
		0xa3,
		0x63, 'f', 'm', 't',
		0x64, 'n', 'o', 'n', 'e',
		0x67, 'a', 't', 't', 'S', 't', 'm', 't',
		0xa0,
		0x68, 'a', 'u', 't', 'h', 'D', 'a', 't', 'a',
		0x58, byte(len(auth)),
	}
	return append(out, auth...)
}

func challengeOf(t *testing.T, raw string) (challenge, userID string) {
	t.Helper()
	var body struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	if body.PublicKey.Challenge == "" {
		t.Fatal(raw)
	}
	return body.PublicKey.Challenge, body.PublicKey.User.ID
}
