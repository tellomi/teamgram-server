// Package dh is the Diffie-Hellman parameter set stock Telegram clients use
// for voice and video calls. Clients learn p and g from messages.getDhConfig
// (the same 2048-bit prime as MTProto, generator 3) and the server checks
// every g_a and g_b against that prime before it forwards them.
package dh

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/big"
)

// Version is the messages.dhConfig version. Clients that already have this
// version receive messages.dhConfigNotModified.
const Version int32 = 3

// G is the generator advertised by messages.getDhConfig.
const G int32 = 3

// PrimeHex is the 2048-bit safe prime from Telegram's DH configuration
// (generator 3). It is the same prime the SRP tests in this repo use.
const PrimeHex = "C71CAEB9C6B1C9048E6C522F70F13F73980D40238E3E21C14934D037563D930F48198A0AA7C14058229493D22530F4DBFA336F6E0AC925139543AED44CCE7C3720FD51F69458705AC68CD4FE6B6B13ABDC9746512969328454F18FAF8C595F642477FE96BB2A941D5BCD1D4AC8CC49880708FA9B378E3C4F3A9060BEE67CF9A4A4A695811051907E162753B56B0F6B410DBA74D8A84B2A14B3144E0EF1284754FD17ED950D5965B4B9DD46582DB1178D169C6BC465B0D6FF9CA3928FEF5B9AE4E418FC15E83EBEA0F87FA9FF5EED70050DED2849F47BF959D956850CE929851F0D8115F635B105EE2E4E15D04B2454BF6F4FADF034B10403119CD8E3B92FCC5B"

var (
	prime      *big.Int
	primeBytes []byte
	subExp     *big.Int
	lowBound   *big.Int
	highBound  *big.Int
)

func init() {
	raw, err := hex.DecodeString(PrimeHex)
	if err != nil || len(raw) != 256 {
		panic("dh: prime must be 256 bytes")
	}
	primeBytes = raw
	prime = new(big.Int).SetBytes(raw)
	// Subgroup order q = (p-1)/2. A public value in the subgroup of order q
	// satisfies g_a^q ≡ 1 (mod p).
	subExp = new(big.Int).Rsh(new(big.Int).Sub(prime, big.NewInt(1)), 1)
	lowBound = new(big.Int).Lsh(big.NewInt(1), 2048-64)
	highBound = new(big.Int).Sub(prime, lowBound)
}

// PrimeBytes is the 256-byte big-endian prime placed in messages.dhConfig.
func PrimeBytes() []byte {
	out := make([]byte, len(primeBytes))
	copy(out, primeBytes)
	return out
}

// HashMatches reports whether SHA256(ga) equals the 32-byte hash from
// phone.requestCall. The hash covers the exact bytes the client sent.
func HashMatches(ga, hash []byte) bool {
	if len(hash) != sha256.Size || len(ga) == 0 {
		return false
	}
	sum := sha256.Sum256(ga)
	ok := true
	for i := range sum {
		ok = ok && sum[i] == hash[i]
	}
	return ok
}

// ValidPublic reports whether ga is a safe DH public value for this prime:
// it is in (1, p-1), outside the 2^1984 margin Telegram recommends, and in
// the prime-order subgroup (g_a^((p-1)/2) ≡ 1 mod p).
func ValidPublic(ga []byte) bool {
	if len(ga) == 0 || len(ga) > 256 {
		return false
	}
	g := new(big.Int).SetBytes(ga)
	if g.Cmp(big.NewInt(1)) <= 0 || g.Cmp(prime) >= 0 {
		return false
	}
	if g.Cmp(lowBound) < 0 || g.Cmp(highBound) > 0 {
		return false
	}
	r := new(big.Int).Exp(g, subExp, prime)
	return r.Cmp(big.NewInt(1)) == 0
}

// Shared computes pow(peer, secret, p). Tests use it to check that both
// sides of a call derive the same key. The server never sees the secrets.
func Shared(peerPublic, secret []byte) []byte {
	p := new(big.Int).SetBytes(peerPublic)
	s := new(big.Int).SetBytes(secret)
	k := new(big.Int).Exp(p, s, prime)
	out := make([]byte, 256)
	b := k.Bytes()
	copy(out[256-len(b):], b)
	return out
}

// Public computes pow(g, secret, p) as 256 big-endian bytes, matching what
// a client puts in g_a / g_b.
func Public(secret []byte) []byte {
	g := big.NewInt(int64(G))
	s := new(big.Int).SetBytes(secret)
	v := new(big.Int).Exp(g, s, prime)
	out := make([]byte, 256)
	b := v.Bytes()
	copy(out[256-len(b):], b)
	return out
}

// Fingerprint is the lower 64 bits of SHA1(key), as a signed int64. Clients
// compare this with phoneCall.key_fingerprint. The server stores the value
// the caller sends in phone.confirmCall and does not recompute it.
func Fingerprint(key []byte) int64 {
	sum := sha1.Sum(key)
	return int64(binary.BigEndian.Uint64(sum[sha1.Size-8:]))
}
