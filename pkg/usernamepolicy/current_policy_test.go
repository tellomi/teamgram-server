package usernamepolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// Digest of tellomi/tellomi policy/dist/policy-2026110602.json.
// The embedded copy must stay byte-for-byte identical.
const currentPolicySHA256 = "cd975005f13eca06ea99435b0ab4df32e1daeba9b189793277d90396f946a72c"

func TestCurrentPolicyDigest(t *testing.T) {
	sum := sha256.Sum256(currentPolicy)
	if hex.EncodeToString(sum[:]) != currentPolicySHA256 {
		t.Fatalf("embedded policy-2026110602 sha256 %x", sum)
	}
	e := currentEngine()
	if e.Version() != 2026110602 || e.RuleCount() != 1024 || len(e.allow) != 52 {
		t.Fatalf("version %d rules %d allow %d", e.Version(), e.RuleCount(), len(e.allow))
	}
}

// Every appendix C / D-56 overlay term must still be refused on the fields
// and match kinds that overlay-d56.json publishes. The runtime file may be
// stricter. It must not allowlist those terms.
func TestOverlayD56NotRelaxed(t *testing.T) {
	e := runtimeEngine()
	global := []string{"global"}
	for _, spec := range overlaySpecs() {
		for _, word := range spec.words {
			norm := Normalize(word)
			for _, entry := range e.allow {
				if entry.Normalized != norm {
					continue
				}
				for _, field := range spec.fields {
					if containsStr(entry.Fields, field) {
						t.Errorf("allowlist covers overlay %s field %s", word, field)
					}
				}
			}
			for _, field := range spec.fields {
				if e.Check(word, field, global).IsAllowed() {
					t.Errorf("overlay exact %s field %s was allowed", word, field)
				}
			}
			if containsStr(spec.matches, MatchPrefix) {
				name := word + "_01"
				for _, field := range spec.fields {
					if e.Check(name, field, global).IsAllowed() {
						t.Errorf("overlay prefix %s field %s was allowed", name, field)
					}
				}
			}
			if containsStr(spec.matches, MatchSuffix) {
				name := "my_" + word
				for _, field := range spec.fields {
					if e.Check(name, field, global).IsAllowed() {
						t.Errorf("overlay suffix %s field %s was allowed", name, field)
					}
				}
			}
		}
	}
}

func TestCurrentD24AndD56(t *testing.T) {
	e := runtimeEngine()
	global := []string{"global"}
	for _, name := range []string{"admin01", "support88", "weibo88", "admin", "tellomi"} {
		if !e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("%s should be allowed", name)
		}
	}
	if e.Check("admin", FieldDisplayName, global).IsAllowed() {
		t.Fatal("display name admin is not on the username allowlist")
	}
	if e.Check("tellomi", FieldDisplayName, global).IsAllowed() {
		t.Fatal("display name tellomi stays reserved")
	}
	for _, name := range []string{"admin_01", "support_88", "tellomi_xxx", "weibo", "weibo_01", "my_weibo", "wechat", "google", "telegram", "telegram_support"} {
		if e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("%s should be refused", name)
		}
	}
	if e.Check("qq", FieldSlug, global).IsAllowed() {
		t.Fatal("qq slug should be refused")
	}
	if !e.Check("qq", FieldUsername, global).IsAllowed() {
		t.Fatal("qq is too short for a username rule; the username field stays allowed")
	}
}

func TestCnTermStaysInCn(t *testing.T) {
	e := runtimeEngine()
	if e.Check("成克杰", FieldDisplayName, []string{"global"}).Outcome != OutcomeAllowed {
		t.Fatal("cn term applied outside cn")
	}
	if e.Check("成克杰", FieldDisplayName, []string{"cn"}).Outcome != OutcomeRegionalRestricted {
		t.Fatal("成克杰 should be regional_restricted in cn")
	}
	if !e.Check("贪污", FieldDisplayName, []string{"cn"}).IsAllowed() {
		t.Fatal("贪污 was removed from the published cn lexicon")
	}
}
