package discoverauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSignAudienceMatchesClaimOrder(t *testing.T) {
	const exp int64 = 1_760_000_000
	owner := Pseudonym(pseudonymKey(), 42)
	member := Pseudonym(pseudonymKey(), 7)
	token, err := SignAudience(signingKey(), owner, AudienceKindInclude, [][]byte{member}, exp)
	if err != nil {
		t.Fatal(err)
	}
	ownerSub := EncodeSub(owner)
	memberSub := EncodeSub(member)
	body := `{"typ":"tellomi-feed-audience","exp":1760000000,"owner":"` + ownerSub + `","kind":"include","members":["` + memberSub + `"]}`
	payload := base64.RawURLEncoding.EncodeToString([]byte(body))
	signing := headerB64 + "." + payload
	mac := hmac.New(sha256.New, signingKey())
	_, _ = mac.Write([]byte(signing))
	want := signing + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if token != want {
		t.Fatalf("token %s", token)
	}
	claims, err := VerifyAudience(signingKey(), token, time.Unix(exp, 0))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Typ != AudienceTyp || claims.Owner != ownerSub || claims.Kind != AudienceKindInclude || len(claims.Members) != 1 || claims.Members[0] != memberSub || claims.Exp != exp {
		t.Fatalf("%+v", claims)
	}
	if _, err = VerifyAudience(signingKey(), token, time.Unix(exp+61, 0)); err == nil {
		t.Fatal("expired audience token was accepted")
	}
	parts := strings.Split(token, ".")
	parts[2] = parts[2][:len(parts[2])-2] + "aa"
	if _, err = VerifyAudience(signingKey(), strings.Join(parts, "."), time.Unix(exp, 0)); err == nil {
		t.Fatal("tampered audience token was accepted")
	}
	// A credential must not verify as an audience list.
	cred, err := Sign(signingKey(), owner, RegionCN, exp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyAudience(signingKey(), cred, time.Unix(exp, 0)); err == nil {
		t.Fatal("credential was accepted as an audience token")
	}
}

func TestIssueAudienceUsesSamePseudonymAndTenMinutes(t *testing.T) {
	now := time.Date(2026, 11, 4, 1, 2, 3, 400, time.UTC)
	got, err := IssueAudience(signingKey(), pseudonymKey(), 42, AudienceKindExclude, []int64{7, 9}, now)
	if err != nil {
		t.Fatal(err)
	}
	wantExp := now.Add(AudienceTTL).Truncate(time.Second).Unix()
	if got.Expires != wantExp || wantExp-now.Unix() > int64(AudienceMaxTTL/time.Second) {
		t.Fatalf("exp %d now %d", got.Expires, now.Unix())
	}
	claims, err := VerifyAudience(signingKey(), got.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Kind != AudienceKindExclude || claims.Owner != EncodeSub(Pseudonym(pseudonymKey(), 42)) {
		t.Fatalf("%+v", claims)
	}
	if claims.Members[0] != EncodeSub(Pseudonym(pseudonymKey(), 7)) || claims.Members[1] != EncodeSub(Pseudonym(pseudonymKey(), 9)) {
		t.Fatalf("members %v", claims.Members)
	}
	if claims.Owner == EncodeSub(Pseudonym(pseudonymKey(), 7)) {
		t.Fatal("owner pseudonym collided with a member")
	}
}

func TestValidateAudienceRejectsShape(t *testing.T) {
	ids := make([]int64, AudienceMaxMembers)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	if err := ValidateAudience(1000, AudienceKindInclude, ids); err != nil {
		t.Fatal(err)
	}
	ids = append(ids, 9000)
	if err := ValidateAudience(1000, AudienceKindInclude, ids); err != ErrAudienceInvalid {
		t.Fatalf("501: %v", err)
	}
	cases := []struct {
		owner int64
		kind  string
		ids   []int64
	}{
		{1, "friends", []int64{2}},
		{1, "INCLUDE", []int64{2}},
		{1, "", []int64{2}},
		{1, AudienceKindInclude, nil},
		{1, AudienceKindExclude, []int64{}},
		{1, AudienceKindInclude, []int64{1}},
		{1, AudienceKindInclude, []int64{0}},
		{1, AudienceKindInclude, []int64{-3}},
		{1, AudienceKindInclude, []int64{2, 2}},
		{0, AudienceKindInclude, []int64{2}},
	}
	for _, c := range cases {
		if err := ValidateAudience(c.owner, c.kind, c.ids); err != ErrAudienceInvalid {
			t.Fatalf("%+v: %v", c, err)
		}
	}
}
