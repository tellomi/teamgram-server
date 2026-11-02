// password_srp.go is the client half of pkg/srp.
// This module does not import the server module.
//
// The math matches Telegram Android SRPHelper and tdlib:
// passwordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow
// with the 2048-bit modulus official clients hard-code and g = 3.
// https://core.telegram.org/api/srp
package e2e

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"math/big"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// Generator is the SRP generator g.
	Generator = 3
	// PadSize is the field element width in bytes.
	PadSize     = 256
	pbkdf2Iters = 100000
	pbkdf2Len   = 64
)

// Prime is the 2048-bit modulus from https://core.telegram.org/api/srp.
var Prime = []byte{
	0xc7, 0x1c, 0xae, 0xb9, 0xc6, 0xb1, 0xc9, 0x04, 0x8e, 0x6c, 0x52, 0x2f,
	0x70, 0xf1, 0x3f, 0x73, 0x98, 0x0d, 0x40, 0x23, 0x8e, 0x3e, 0x21, 0xc1,
	0x49, 0x34, 0xd0, 0x37, 0x56, 0x3d, 0x93, 0x0f, 0x48, 0x19, 0x8a, 0x0a,
	0xa7, 0xc1, 0x40, 0x58, 0x22, 0x94, 0x93, 0xd2, 0x25, 0x30, 0xf4, 0xdb,
	0xfa, 0x33, 0x6f, 0x6e, 0x0a, 0xc9, 0x25, 0x13, 0x95, 0x43, 0xae, 0xd4,
	0x4c, 0xce, 0x7c, 0x37, 0x20, 0xfd, 0x51, 0xf6, 0x94, 0x58, 0x70, 0x5a,
	0xc6, 0x8c, 0xd4, 0xfe, 0x6b, 0x6b, 0x13, 0xab, 0xdc, 0x97, 0x46, 0x51,
	0x29, 0x69, 0x32, 0x84, 0x54, 0xf1, 0x8f, 0xaf, 0x8c, 0x59, 0x5f, 0x64,
	0x24, 0x77, 0xfe, 0x96, 0xbb, 0x2a, 0x94, 0x1d, 0x5b, 0xcd, 0x1d, 0x4a,
	0xc8, 0xcc, 0x49, 0x88, 0x07, 0x08, 0xfa, 0x9b, 0x37, 0x8e, 0x3c, 0x4f,
	0x3a, 0x90, 0x60, 0xbe, 0xe6, 0x7c, 0xf9, 0xa4, 0xa4, 0xa6, 0x95, 0x81,
	0x10, 0x51, 0x90, 0x7e, 0x16, 0x27, 0x53, 0xb5, 0x6b, 0x0f, 0x6b, 0x41,
	0x0d, 0xba, 0x74, 0xd8, 0xa8, 0x4b, 0x2a, 0x14, 0xb3, 0x14, 0x4e, 0x0e,
	0xf1, 0x28, 0x47, 0x54, 0xfd, 0x17, 0xed, 0x95, 0x0d, 0x59, 0x65, 0xb4,
	0xb9, 0xdd, 0x46, 0x58, 0x2d, 0xb1, 0x17, 0x8d, 0x16, 0x9c, 0x6b, 0xc4,
	0x65, 0xb0, 0xd6, 0xff, 0x9c, 0xa3, 0x92, 0x8f, 0xef, 0x5b, 0x9a, 0xe4,
	0xe4, 0x18, 0xfc, 0x15, 0xe8, 0x3e, 0xbe, 0xa0, 0xf8, 0x7f, 0xa9, 0xff,
	0x5e, 0xed, 0x70, 0x05, 0x0d, 0xed, 0x28, 0x49, 0xf4, 0x7b, 0xf9, 0x59,
	0xd9, 0x56, 0x85, 0x0c, 0xe9, 0x29, 0x85, 0x1f, 0x0d, 0x81, 0x15, 0xf6,
	0x35, 0xb1, 0x05, 0xee, 0x2e, 0x4e, 0x15, 0xd0, 0x4b, 0x24, 0x54, 0xbf,
	0x6f, 0x4f, 0xad, 0xf0, 0x34, 0xb1, 0x04, 0x03, 0x11, 0x9c, 0xd8, 0xe3,
	0xb9, 0x2f, 0xcc, 0x5b,
}

var prime = new(big.Int).SetBytes(Prime)

func init() {
	if len(Prime) != PadSize {
		panic("srp: prime must be 256 bytes")
	}
}

// Pad left-pads a big-endian field element to 256 bytes.
// A value that does not fit the field returns nil.
func Pad(b []byte) []byte {
	b = trimLeftZero(b)
	if len(b) > PadSize {
		return nil
	}
	out := make([]byte, PadSize)
	copy(out[PadSize-len(b):], b)
	return out
}

func trimLeftZero(b []byte) []byte {
	i := 0
	for i < len(b)-1 && b[i] == 0 {
		i++
	}
	return b[i:]
}

// PasswordHash is x = PH2(password, salt1, salt2) from SRPHelper.getX.
// The first digest is SHA256(salt1 | password | salt1), not salt2.
func PasswordHash(password, salt1, salt2 []byte) []byte {
	h := sha256.Sum256(concat(salt1, password, salt1))
	h = sha256.Sum256(concat(salt2, h[:], salt2))
	dk := pbkdf2.Key(h[:], salt1, pbkdf2Iters, pbkdf2Len, sha512.New)
	out := sha256.Sum256(concat(salt2, dk, salt2))
	return out[:]
}

// Verifier is v = g^x mod p, 256 bytes, big-endian.
func Verifier(password, salt1, salt2 []byte) []byte {
	x := new(big.Int).SetBytes(PasswordHash(password, salt1, salt2))
	v := new(big.Int).Exp(big.NewInt(Generator), x, prime)
	return Pad(v.Bytes())
}

// Challenge is one server attempt: private b and public B = (k*v + g^b) mod p.
type Challenge struct {
	BPriv []byte
	BPub  []byte
}

// NewChallenge picks b and computes B for the stored verifier.
func NewChallenge(verifier []byte) (Challenge, error) {
	v := new(big.Int).SetBytes(verifier)
	if v.Sign() <= 0 || v.Cmp(prime) >= 0 {
		return Challenge{}, errBadVerifier
	}
	k := hashInt(Prime, padG())
	for i := 0; i < 8; i++ {
		raw := make([]byte, PadSize)
		if _, err := rand.Read(raw); err != nil {
			return Challenge{}, err
		}
		b := new(big.Int).SetBytes(raw)
		b.Mod(b, prime)
		if b.Sign() == 0 {
			continue
		}
		gb := new(big.Int).Exp(big.NewInt(Generator), b, prime)
		if !goodElement(gb) {
			continue
		}
		kv := new(big.Int).Mul(k, v)
		kv.Mod(kv, prime)
		B := new(big.Int).Add(kv, gb)
		B.Mod(B, prime)
		if !goodElement(B) {
			continue
		}
		return Challenge{BPriv: Pad(b.Bytes()), BPub: Pad(B.Bytes())}, nil
	}
	return Challenge{}, errNoChallenge
}

// ClientProof builds A and M1 the way Telegram Android SRPHelper.startCheck does.
// a is the 256-byte client secret. B is the server's padded public value.
func ClientProof(password, salt1, salt2, a, B []byte) (A, M1 []byte, err error) {
	if len(a) == 0 || len(B) == 0 {
		return nil, nil, errBadVerifier
	}
	p := new(big.Int).Set(prime)
	g := big.NewInt(Generator)
	x := new(big.Int).SetBytes(PasswordHash(password, salt1, salt2))
	k := hashInt(Prime, padG())
	aInt := new(big.Int).SetBytes(a)
	Aint := new(big.Int).Exp(g, aInt, p)
	A = Pad(Aint.Bytes())
	Bint := new(big.Int).SetBytes(B)
	if !goodElement(Bint) {
		return nil, nil, errBadVerifier
	}
	Bp := Pad(Bint.Bytes())
	u := new(big.Int).SetBytes(sha256Sum(A, Bp))
	if u.Sign() == 0 {
		return nil, nil, errBadVerifier
	}
	gx := new(big.Int).Exp(g, x, p)
	kgx := new(big.Int).Mul(k, gx)
	kgx.Mod(kgx, p)
	t := new(big.Int).Sub(Bint, kgx)
	if t.Sign() < 0 {
		t.Add(t, p)
	}
	if !goodElement(t) {
		return nil, nil, errBadVerifier
	}
	exp := new(big.Int).Add(aInt, new(big.Int).Mul(u, x))
	S := new(big.Int).Exp(t, exp, p)
	K := sha256Sum(Pad(S.Bytes()))
	return A, m1(salt1, salt2, A, Bp, K), nil
}

// Verify reports whether M1 proves knowledge of the password for this attempt.
// The comparison is constant-time. A malformed A or a zero u fails closed.
func Verify(verifier, salt1, salt2, bPriv, BPub, A, M1 []byte) bool {
	if len(M1) != sha256.Size || len(verifier) == 0 || len(bPriv) == 0 || len(BPub) == 0 || len(A) == 0 {
		return false
	}
	Ap := Pad(A)
	Bp := Pad(BPub)
	if Ap == nil || Bp == nil {
		return false
	}
	a := new(big.Int).SetBytes(Ap)
	if !goodElement(a) {
		return false
	}
	u := new(big.Int).SetBytes(sha256Sum(Ap, Bp))
	if u.Sign() == 0 {
		return false
	}
	v := new(big.Int).SetBytes(verifier)
	b := new(big.Int).SetBytes(bPriv)
	vu := new(big.Int).Exp(v, u, prime)
	base := new(big.Int).Mul(a, vu)
	base.Mod(base, prime)
	if base.Sign() == 0 {
		return false
	}
	S := new(big.Int).Exp(base, b, prime)
	if S.Sign() == 0 {
		return false
	}
	Sp := Pad(S.Bytes())
	K := sha256Sum(Sp)
	want := m1(salt1, salt2, Ap, Bp, K)
	return subtle.ConstantTimeCompare(want, M1) == 1
}

func m1(salt1, salt2, A, B, K []byte) []byte {
	ph := sha256.Sum256(Prime)
	gh := sha256.Sum256(padG())
	xor := make([]byte, len(ph))
	for i := range ph {
		xor[i] = ph[i] ^ gh[i]
	}
	return sha256Sum(xor, sha256Sum(salt1), sha256Sum(salt2), A, B, K)
}

func padG() []byte {
	return Pad(big.NewInt(Generator).Bytes())
}

func hashInt(parts ...[]byte) *big.Int {
	return new(big.Int).SetBytes(sha256Sum(parts...))
}

func sha256Sum(parts ...[]byte) []byte {
	sum := sha256.Sum256(concat(parts...))
	return sum[:]
}

func concat(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func goodElement(n *big.Int) bool {
	if n == nil || n.Cmp(big.NewInt(1)) <= 0 {
		return false
	}
	limit := new(big.Int).Sub(prime, big.NewInt(1))
	return n.Cmp(limit) < 0
}

type srpError string

func (e srpError) Error() string { return string(e) }

const (
	errBadVerifier = srpError("srp: verifier out of range")
	errNoChallenge = srpError("srp: could not draw a challenge")
)
