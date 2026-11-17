package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMergePhonePrivacyExceptionLetsThatUserSeeTheNumber(t *testing.T) {
	base := []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowUsers(&mtproto.PrivacyRule{Users: []int64{8, 9}}).To_PrivacyRule(),
	}
	if mtproto.EvaluatePrivacy(base, mtproto.PrivacySubject{UserID: 9}) {
		t.Fatal("disallow all already revealed the number")
	}
	next := mergePhonePrivacyException(base, 9)
	if !mtproto.EvaluatePrivacy(next, mtproto.PrivacySubject{UserID: 9}) {
		t.Fatal("exception did not reveal the number")
	}
	if mtproto.EvaluatePrivacy(next, mtproto.PrivacySubject{UserID: 4}) {
		t.Fatal("exception revealed a different user")
	}
	if mtproto.EvaluatePrivacy(next, mtproto.PrivacySubject{UserID: 8}) {
		t.Fatal("a different disallow user was dropped")
	}
	again := mergePhonePrivacyException(next, 9)
	var n int
	for _, rule := range again {
		if rule.GetPredicateName() != mtproto.Predicate_privacyValueAllowUsers {
			continue
		}
		for _, id := range rule.GetUsers() {
			if id == 9 {
				n++
			}
		}
	}
	if n != 1 {
		t.Fatalf("peer listed %d times", n)
	}
	if same := mergePhonePrivacyException(base, 0); len(same) != len(base) {
		t.Fatal("peer 0 changed the rules")
	}
}
