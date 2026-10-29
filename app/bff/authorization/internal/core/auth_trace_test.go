package core

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestNewUserSignInLayer232(t *testing.T) {
	authz, err := newUserSignInResult(232)
	if err != nil {
		t.Fatal(err)
	}
	if authz.GetPredicateName() != mtproto.Predicate_auth_authorizationSignUpRequired {
		t.Fatalf("predicate %s", authz.GetPredicateName())
	}
	tos := authz.GetTermsOfService()
	if tos == nil || !strings.Contains(tos.GetText(), "用户服务协议") || !strings.Contains(tos.GetText(), "隐私政策") {
		t.Fatalf("signup terms: %v", tos)
	}
	if strings.Contains(tos.GetText(), "https://") {
		t.Fatalf("signup body is raw urls: %s", tos.GetText())
	}
	ents := tos.GetEntities()
	if len(ents) != 2 || ents[0].GetUrl() != brand.TermsURL || ents[1].GetUrl() != brand.PrivacyURL {
		t.Fatalf("entities %+v", ents)
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
