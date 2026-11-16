package usernamepolicy

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy/lexiconsrc"
)

// The pinned konsheng and fwwdn category files classify to the 391 cn
// politics rules already in policy-2026110602. Deferred and dropped lines
// stay out. This is a coverage check, not a new word list.
func TestMatureLexiconMatchesPublishedPolicy(t *testing.T) {
	rep, err := lexiconsrc.Classify()
	if err != nil {
		t.Fatal(err)
	}
	pinyin := map[string]struct{}{}
	for _, term := range rep.Pinyin {
		pinyin[term] = struct{}{}
	}
	admit := map[string]struct{}{}
	for _, term := range rep.Admit {
		admit[term] = struct{}{}
	}
	got := map[string]rule{}
	for _, rule := range currentEngine().rules {
		if !strings.Contains(rule.Source, "regions/cn/politics.toml") {
			continue
		}
		if _, dup := got[rule.Term]; dup {
			t.Fatalf("duplicate term %s", rule.Term)
		}
		got[rule.Term] = rule
	}
	if len(got) != lexiconsrc.PolicyRuleCount {
		t.Fatalf("published cn politics rules %d", len(got))
	}
	for term := range admit {
		rule, ok := got[term]
		if !ok {
			t.Fatalf("admit %s missing from policy-2026110602", term)
		}
		assertSpec(t, rule, lexiconsrc.SpecFor(term, false))
	}
	for term := range pinyin {
		rule, ok := got[term]
		if !ok {
			t.Fatalf("pinyin %s missing from policy-2026110602", term)
		}
		assertSpec(t, rule, lexiconsrc.SpecFor(term, true))
	}
	for term, rule := range got {
		_, a := admit[term]
		_, p := pinyin[term]
		if !a && !p {
			t.Fatalf("policy term %s (%s) is not an admitted source line or its pinyin", term, rule.ID)
		}
	}
	for _, term := range append(append([]string{}, rep.DeferBoth...), "共产党", "习近平", "贪污", "双开") {
		if _, ok := got[term]; ok {
			t.Fatalf("deferred or dropped %s is a published rule", term)
		}
	}
	for _, term := range cnUsernameGapTerms {
		if _, ok := admit[term]; ok {
			t.Fatalf("gap term %s is an admitted source line", term)
		}
		if _, ok := pinyin[term]; ok {
			t.Fatalf("gap term %s is an admitted pinyin", term)
		}
	}
}

func TestMatureLexiconBehavior(t *testing.T) {
	rep, err := lexiconsrc.Classify()
	if err != nil {
		t.Fatal(err)
	}
	e := runtimeEngine()
	cn := []string{"global", "cn"}
	global := []string{"global"}
	pinyin := map[string]struct{}{}
	for _, term := range rep.Pinyin {
		pinyin[term] = struct{}{}
	}
	for _, term := range rep.Admit {
		if e.Check(term, FieldDisplayName, cn).IsAllowed() {
			t.Fatalf("admit display %s was allowed", term)
		}
		if e.Check(term, FieldDisplayName, global).Outcome != OutcomeAllowed {
			t.Fatalf("admit display %s applied outside cn", term)
		}
	}
	for _, term := range rep.Pinyin {
		if e.Check(term, FieldUsername, cn).IsAllowed() {
			t.Fatalf("pinyin username %s was allowed", term)
		}
		if !e.Check(term, FieldUsername, global).IsAllowed() {
			t.Fatalf("pinyin username %s denied outside cn", term)
		}
		if e.Check(strings.ToUpper(term), FieldUsername, cn).IsAllowed() {
			t.Fatalf("pinyin case %s was allowed", term)
		}
	}
	// Latin lines that overlap the excluded Tencent file. A global reserved
	// name may still refuse them. The cn politics rules must not.
	for _, term := range []string{"youtube", "wikipedia", "boycott", "taiwan", "hujintao", "gfw"} {
		for _, field := range []string{FieldUsername, FieldDisplayName} {
			v := e.Check(term, field, cn)
			if strings.Contains(v.Source, "regions/cn") || strings.HasPrefix(v.RuleID, "supplement/cn-username:") {
				t.Fatalf("deferred latin %s %s hit %s", field, term, v.RuleID)
			}
		}
	}
	for _, term := range []string{
		"刘瑞龙", "周建南", "李干成", "薄熙", "贺子珍", "黄敬", "黄镇",
		"贪污", "双开", "共产党", "习近平",
	} {
		if !e.Check(term, FieldDisplayName, cn).IsAllowed() {
			t.Fatalf("deferred display %s was blocked", term)
		}
	}
	// Skeletons shorter than 8 stay on the published exact rule. The digit
	// bypass is not extended to them.
	for _, term := range []string{"lvdebin", "sunyu", "tan20yi"} {
		if e.Check(term, FieldUsername, cn).IsAllowed() {
			t.Fatalf("short pinyin %s lost its exact rule", term)
		}
		if _, ok := pinyin[term]; !ok {
			t.Fatalf("short pinyin %s is not an admitted reading", term)
		}
		withDigit := term + "1"
		if term == "tan20yi" {
			withDigit = "tan20yi9"
		}
		v := e.Check(withDigit, FieldUsername, cn)
		if !v.IsAllowed() {
			t.Fatalf("short pinyin digit form %s hit %s", withDigit, v.RuleID)
		}
	}
}

func TestCnUsernameSupplementEdges(t *testing.T) {
	e := runtimeEngine()
	cn := []string{"global", "cn"}
	curated := 0
	for _, term := range cnSupplementTerms() {
		if term.gap {
			continue
		}
		curated++
		upper := strings.ToUpper(term.term)
		if e.Check(upper, FieldUsername, cn).IsAllowed() {
			t.Fatalf("case %s was allowed", upper)
		}
		digit := term.term + "7"
		v := e.Check(digit, FieldUsername, cn)
		if v.IsAllowed() || !strings.HasPrefix(v.RuleID, "supplement/cn-username:") {
			t.Fatalf("digit %s verdict %+v", digit, v)
		}
		buried := "zz" + term.sk + "qq"
		if !e.Check(buried, FieldUsername, cn).IsAllowed() {
			t.Fatalf("buried %s was blocked", buried)
		}
		if utf8.RuneCountInString(term.sk) < cnUsernameSupplementMinRunes {
			t.Fatalf("curated skeleton shorter than the floor: %s", term.sk)
		}
	}
	if curated != 188 {
		t.Fatalf("curated %d", curated)
	}
}

func assertSpec(t *testing.T, rule rule, spec lexiconsrc.TermSpec) {
	t.Helper()
	if strings.Join(rule.Matches, ",") != strings.Join(spec.Matches, ",") {
		t.Fatalf("%s matches %v want %v", rule.Term, rule.Matches, spec.Matches)
	}
	if strings.Join(rule.Fields, ",") != strings.Join(spec.Fields, ",") {
		t.Fatalf("%s fields %v want %v", rule.Term, rule.Fields, spec.Fields)
	}
	if len(rule.Regions) != 1 || rule.Regions[0] != "cn" {
		t.Fatalf("%s regions %v", rule.Term, rule.Regions)
	}
	if rule.Outcome != OutcomeRegionalRestricted {
		t.Fatalf("%s outcome %s", rule.Term, rule.Outcome)
	}
}
