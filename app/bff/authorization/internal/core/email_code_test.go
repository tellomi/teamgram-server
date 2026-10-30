package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestNormalizeEmailLogin(t *testing.T) {
	got, err := normalizeEmailLogin(" User@Tellomi.app ")
	if err != nil || got != "user@tellomi.app" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := normalizeEmailLogin("not-an-email"); err != mtproto.ErrEmailInvalid {
		t.Fatal(err)
	}
	if _, err := normalizeEmailLogin("a@b"); err != mtproto.ErrEmailInvalid {
		t.Fatal(err)
	}
}

func TestEmailLoginCode(t *testing.T) {
	code, err := emailLoginCode(&mtproto.TLAuthSignIn{PhoneCode_STRING: "012345"})
	if err != nil || code != "012345" {
		t.Fatalf("%q %v", code, err)
	}
	code, err = emailLoginCode(&mtproto.TLAuthSignIn{
		EmailVerification: &mtproto.EmailVerification{
			PredicateName: mtproto.Predicate_emailVerificationCode,
			Code:          "654321",
		},
	})
	if err != nil || code != "654321" {
		t.Fatalf("verification code %q %v", code, err)
	}
	_, err = emailLoginCode(&mtproto.TLAuthSignIn{
		EmailVerification: &mtproto.EmailVerification{
			PredicateName: mtproto.Predicate_emailVerificationGoogle,
			Token:         "tok",
		},
	})
	if err != mtproto.ErrEmailInvalid {
		t.Fatal(err)
	}
	_, err = emailLoginCode(&mtproto.TLAuthSignIn{
		EmailVerification: &mtproto.EmailVerification{
			PredicateName: mtproto.Predicate_emailVerificationApple,
			Token:         "tok",
		},
	})
	if err != mtproto.ErrEmailInvalid {
		t.Fatal(err)
	}
	_, err = emailLoginCode(&mtproto.TLAuthSignIn{})
	if err != mtproto.ErrPhoneCodeEmpty {
		t.Fatal(err)
	}
}

func TestStoredCodeMatches(t *testing.T) {
	if !storedCodeMatches("012345", "012345") {
		t.Fatal("expected match")
	}
	if storedCodeMatches("012345", "012346") || storedCodeMatches("012345", "12345") || storedCodeMatches("", "012345") {
		t.Fatal("expected mismatch")
	}
}
