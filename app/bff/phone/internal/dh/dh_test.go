package dh

import (
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

func TestPrimeAndPublicValue(t *testing.T) {
	if len(PrimeBytes()) != 256 || G != 3 || Version != 3 {
		t.Fatalf("prime %d g %d version %d", len(PrimeBytes()), G, Version)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	secret[0] |= 0x80
	ga := Public(secret)
	if !ValidPublic(ga) {
		t.Fatal("client-style public value rejected")
	}
	sum := sha256.Sum256(ga)
	if !HashMatches(ga, sum[:]) {
		t.Fatal("hash")
	}
	if HashMatches(ga, sum[:31]) || ValidPublic([]byte{1}) || ValidPublic(nil) {
		t.Fatal("weak values accepted")
	}
}

func TestBothSidesAgree(t *testing.T) {
	a := make([]byte, 32)
	b := make([]byte, 32)
	_, _ = rand.Read(a)
	_, _ = rand.Read(b)
	a[0] |= 0x40
	b[0] |= 0x40
	ga := Public(a)
	gb := Public(b)
	if !ValidPublic(ga) || !ValidPublic(gb) {
		t.Fatal("public")
	}
	keyA := Shared(gb, a)
	keyB := Shared(ga, b)
	if Fingerprint(keyA) != Fingerprint(keyB) || len(keyA) != 256 {
		t.Fatalf("keys disagree %x %x", keyA[:8], keyB[:8])
	}
}
