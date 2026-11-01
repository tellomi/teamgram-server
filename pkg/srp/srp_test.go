package srp

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"math/big"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

// clientX is SRPHelper.getX, written out here so a server-side edit cannot
// silently agree with itself.
// SHA256(salt1 | password | salt1), then SHA256(salt2 | that | salt2),
// PBKDF2-HMAC-SHA512 100000 times with salt1, then SHA256(salt2 | dk | salt2).
func clientX(password, salt1, salt2 []byte) []byte {
	h := sha256.Sum256(append(append(append([]byte{}, salt1...), password...), salt1...))
	h = sha256.Sum256(append(append(append([]byte{}, salt2...), h[:]...), salt2...))
	dk := pbkdf2.Key(h[:], salt1, 100000, 64, sha512.New)
	out := sha256.Sum256(append(append(append([]byte{}, salt2...), dk...), salt2...))
	return out[:]
}

func clientPad(b []byte) []byte {
	b = append([]byte(nil), b...)
	for len(b) > 1 && b[0] == 0 {
		b = b[1:]
	}
	if len(b) > 256 {
		return nil
	}
	out := make([]byte, 256)
	copy(out[256-len(b):], b)
	return out
}

// clientProof is SRPHelper.startCheck for a chosen secret a.
func clientProof(password, salt1, salt2, aBytes, B []byte) (A, M1 []byte) {
	p := new(big.Int).SetBytes(Prime)
	g := big.NewInt(Generator)
	gBytes := clientPad(g.Bytes())
	x := new(big.Int).SetBytes(clientX(password, salt1, salt2))
	k := new(big.Int).SetBytes(sum256(Prime, gBytes))
	a := new(big.Int).SetBytes(aBytes)
	Aint := new(big.Int).Exp(g, a, p)
	A = clientPad(Aint.Bytes())
	Bint := new(big.Int).SetBytes(B)
	u := new(big.Int).SetBytes(sum256(A, clientPad(Bint.Bytes())))
	gx := new(big.Int).Exp(g, x, p)
	kgx := new(big.Int).Mod(new(big.Int).Mul(k, gx), p)
	t := new(big.Int).Sub(Bint, kgx)
	if t.Sign() < 0 {
		t.Add(t, p)
	}
	exp := new(big.Int).Add(a, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(t, exp, p)
	K := sum256(clientPad(S.Bytes()))
	ph := sha256.Sum256(Prime)
	gh := sha256.Sum256(gBytes)
	xor := make([]byte, len(ph))
	for i := range ph {
		xor[i] = ph[i] ^ gh[i]
	}
	M1 = sum256(xor, sum256(salt1), sum256(salt2), A, clientPad(Bint.Bytes()), K)
	return A, M1
}

func sum256(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write(p)
	}
	return h.Sum(nil)
}

func TestPasswordHashMatchesClientKDF(t *testing.T) {
	password := []byte("cloud-password")
	salt1 := bytesRepeat(0x11, 8+32)
	salt2 := bytesRepeat(0x22, 16)
	if got, want := PasswordHash(password, salt1, salt2), clientX(password, salt1, salt2); string(got) != string(want) {
		t.Fatal("server KDF drifted from SRPHelper.getX")
	}
	v := Verifier(password, salt1, salt2)
	if len(v) != PadSize || v[0] == 0 && v[1] == 0 && v[2] == 0 {
		t.Fatalf("verifier %d bytes", len(v))
	}
}

func TestVerifyAcceptsClientProof(t *testing.T) {
	password := []byte("correct horse")
	salt1 := append(bytesRepeat(0x31, 8), bytesRepeat(0xab, 32)...)
	salt2 := bytesRepeat(0x42, 16)
	v := Verifier(password, salt1, salt2)
	ch, err := NewChallenge(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.BPub) != PadSize || len(ch.BPriv) != PadSize {
		t.Fatalf("challenge %d %d", len(ch.BPriv), len(ch.BPub))
	}
	a := bytesRepeat(0x5a, 32)
	a = append(make([]byte, 224), a...)
	A, M1 := clientProof(password, salt1, salt2, a, ch.BPub)
	if !Verify(v, salt1, salt2, ch.BPriv, ch.BPub, A, M1) {
		t.Fatal("server rejected a client proof")
	}
	gotA, gotM, err := ClientProof(password, salt1, salt2, a, ch.BPub)
	if err != nil || !bytes.Equal(gotA, A) || !bytes.Equal(gotM, M1) {
		t.Fatal("ClientProof drifted from the independent client")
	}
	bad := append([]byte(nil), M1...)
	bad[0] ^= 0xff
	if Verify(v, salt1, salt2, ch.BPriv, ch.BPub, A, bad) {
		t.Fatal("tampered M1 was accepted")
	}
	other := Verifier([]byte("wrong"), salt1, salt2)
	_, wrong := clientProof([]byte("wrong"), salt1, salt2, a, ch.BPub)
	if Verify(v, salt1, salt2, ch.BPriv, ch.BPub, A, wrong) || Verify(other, salt1, salt2, ch.BPriv, ch.BPub, A, M1) {
		t.Fatal("wrong password was accepted")
	}
	if Verify(v, salt1, salt2, ch.BPriv, ch.BPub, []byte{1}, M1) {
		t.Fatal("A = 1 was accepted")
	}
	zero := make([]byte, sha256.Size)
	if Verify(v, salt1, salt2, ch.BPriv, ch.BPub, A, zero) {
		t.Fatal("zero M1 was accepted")
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
