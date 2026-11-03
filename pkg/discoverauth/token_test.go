package discoverauth

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// engineCredential is content-engine auth.Sign at commit 4483302, copied from
// Signal-Server TellomiFeedTokensTest.ENGINE_CREDENTIAL. Signing key is
// 0x00..0x1f, sub is 0x00..0x0f, rgn is cn, exp is 1760000000.
const engineCredential = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJBQUVDQXdRRkJnY0lDUW9MREEwT0R3IiwicmduIjoiY24iLCJleHAiOjE3NjAwMDAwMDB9.fBe1RxoGuuEYQxttOY8NEojzB2ZvOLzCqo20itIlA4c"

func signingKey() []byte   { return seq(0, 32) }
func pseudonymKey() []byte { return seq(100, 32) }

func seq(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}

func TestSignMatchesEngineAuth(t *testing.T) {
	token, err := Sign(signingKey(), seq(0, 16), RegionCN, 1_760_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if token != engineCredential {
		t.Fatalf("token differs from content-engine auth.Sign")
	}
	claims, err := Verify(signingKey(), token, time.Unix(1_760_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Rgn != RegionCN || claims.Exp != 1_760_000_000 {
		t.Fatalf("claims %+v", claims)
	}
	raw, err := base64.RawURLEncoding.DecodeString(claims.Sub)
	if err != nil || len(raw) != 16 || raw[0] != 0 || raw[15] != 15 {
		t.Fatalf("sub %x err %v", raw, err)
	}
}

func TestVerifyRejectsTamperExpiryAndHeader(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token, err := Sign(signingKey(), seq(0, 16), RegionGlobal, now.Add(time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(signingKey(), token, now); err != nil {
		t.Fatal(err)
	}
	// Inside the 60s leeway, still accepted.
	if _, err = Verify(signingKey(), token, time.Unix(now.Add(time.Hour).Unix()+60, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(signingKey(), token, time.Unix(now.Add(time.Hour).Unix()+61, 0)); err == nil {
		t.Fatal("expired token was accepted")
	}
	parts := strings.Split(token, ".")
	parts[2] = parts[2][:len(parts[2])-2] + "aa"
	if _, err = Verify(signingKey(), strings.Join(parts, "."), now); err == nil {
		t.Fatal("tampered signature was accepted")
	}
	parts = strings.Split(token, ".")
	parts[0] = "e30"
	if _, err = Verify(signingKey(), strings.Join(parts, "."), now); err == nil {
		t.Fatal("foreign header was accepted")
	}
	if _, err = Verify(seq(9, 32), token, now); err == nil {
		t.Fatal("wrong key was accepted")
	}
}

func TestPseudonymIsStableTruncatedHMAC(t *testing.T) {
	const userID = int64(42)
	a := Pseudonym(pseudonymKey(), userID)
	b := Pseudonym(pseudonymKey(), userID)
	if len(a) != 16 || hex.EncodeToString(a) != hex.EncodeToString(b) {
		t.Fatalf("unstable %x %x", a, b)
	}
	// HMAC-SHA256(key = 0x64..0x83, "42")[:16]
	const pseudonym42 = "cc2345f257bf3b09e6715394b26298f4"
	if got := hex.EncodeToString(a); got != pseudonym42 {
		t.Fatalf("pseudonym %s", got)
	}
	other := Pseudonym(seq(7, 32), userID)
	if hex.EncodeToString(other) == hex.EncodeToString(a) {
		t.Fatal("pseudonym ignored the key")
	}
	if hex.EncodeToString(Pseudonym(pseudonymKey(), 43)) == hex.EncodeToString(a) {
		t.Fatal("pseudonym ignored the user id")
	}
}

func TestIssueBindsUserRegionAndVerifies(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 1, 500, time.UTC)
	cred, err := Issue(signingKey(), pseudonymKey(), 42, "CN", "14155552671", false, now)
	if err != nil {
		t.Fatal(err)
	}
	wantExp := now.Add(CredentialTTL).Truncate(time.Second).Unix()
	if cred.Region != RegionCN || cred.Expires != wantExp {
		t.Fatalf("%+v", cred)
	}
	claims, err := Verify(signingKey(), cred.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	wantSub := EncodeSub(Pseudonym(pseudonymKey(), 42))
	if claims.Sub != wantSub || claims.Rgn != RegionCN || claims.Exp != wantExp {
		t.Fatalf("claims %+v", claims)
	}
	over, err := Issue(signingKey(), pseudonymKey(), 7, "", "14155552671", false, now)
	if err != nil || over.Region != RegionGlobal {
		t.Fatalf("%+v %v", over, err)
	}
	if _, err = Issue(signingKey(), pseudonymKey(), 0, "CN", "", false, now); err == nil {
		t.Fatal("user 0 issued a token")
	}
}
