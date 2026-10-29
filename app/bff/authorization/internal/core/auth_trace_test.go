package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestNewUserSignInLayer232(t *testing.T) {
	authz, err := newUserSignInResult(232)
	if err != nil {
		t.Fatal(err)
	}
	if authz.GetPredicateName() != mtproto.Predicate_auth_authorizationSignUpRequired {
		t.Fatalf("predicate %s", authz.GetPredicateName())
	}
	if _, err := newUserSignInResult(103); err == nil || rpcErrorCode(err) != "PHONE_NUMBER_UNOCCUPIED" {
		t.Fatalf("old layer: %v", err)
	}
}

func TestMaskPhoneAndHash(t *testing.T) {
	if got := maskPhone("8613800138000"); got == "8613800138000" || len(got) < 6 {
		t.Fatalf("phone not masked: %s", got)
	}
	if got := hashPrefix("abcdef0123456789"); got != "abcdef01" {
		t.Fatalf("hash prefix %s", got)
	}
}
