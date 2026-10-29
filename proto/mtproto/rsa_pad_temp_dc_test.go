package mtproto

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/teamgram/proto/mtproto/crypto"
)

func TestRSAPadAcceptsTempDC(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key := crypto.NewRSACryptorFromKey(priv)
	inner := MakeTLPQInnerDataTempDc(&P_QInnerData{
		Pq:          string([]byte{0x17, 0xED}),
		P:           string([]byte{0x49}),
		Q:           string([]byte{0x53}),
		Nonce:       bytesRepeat(16, 1),
		ServerNonce: bytesRepeat(16, 2),
		NewNonce:    bytesRepeat(32, 3),
		Dc:          2,
		ExpiresIn:   86400,
	})
	buf := NewEncodeBuf(256)
	if err := inner.Encode(buf, 0); err != nil {
		t.Fatal(err)
	}
	plain := append([]byte(nil), buf.GetBuf()...)
	enc, err := crypto.EncryptRSAPad(&priv.PublicKey, plain, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(enc) != 256 {
		t.Fatalf("ciphertext %d", len(enc))
	}
	got, err := crypto.DecryptPQInner(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	dbuf := NewDecodeBuf(got)
	obj := dbuf.Object()
	if err := dbuf.GetError(); err != nil {
		t.Fatal(err)
	}
	dc, ok := obj.(*TLPQInnerDataTempDc)
	if !ok {
		t.Fatalf("decoded %T", obj)
	}
	if dc.GetExpiresIn() != 86400 || dc.GetDc() != 2 {
		t.Fatalf("expires_in=%d dc=%d", dc.GetExpiresIn(), dc.GetDc())
	}
}

func bytesRepeat(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
