package usernamepolicy

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// cnUsernameGapTerms are not produced by the admission rules in
// pkg/usernamepolicy/lexiconsrc. gongchandang is an exact line of the
// MIT-licensed konsheng Vocabulary/贪腐词库.txt and also an exact line of
// the excluded Vocabulary/零时-Tencent.txt, so D-25 defers it. xijinping
// is not a line of the three vendored category files; it occurs in that
// excluded Tencent file. Both stay as exceptions from the earlier commit,
// on the same cn fields as the other supplement terms. Bare Han nicknames
// 共产党 and 习近平 stay allowed.
// Do not append another term here. A new admit has to come through the
// pinned lists and a new policy JSON. The other supplement terms are the
// latin cn username rules already in policy-2026110602.
//
// Upstream match modes still miss a lookalike glued to digits: PREFIX and
// SUFFIX stop at a letter or digit, and CONFUSABLE compares the whole
// skeleton. For cn username fields only, this supplement folds the
// skeleton, drops remaining digits, and then accepts an exact, prefix, or
// suffix hit. A term buried in the middle (notgongchandangx) is not a hit.
// Skeletons shorter than 8 are ignored, the same floor the upstream build
// uses before it will attach PREFIX/SUFFIX to a latin term.
//
// Global-only checks are unchanged. must-allow still has
// "username global xijinping". Entry points pass global and cn.
const cnUsernameSupplementSource = "teamgram-server/pkg/usernamepolicy/cn_username_supplement.go"

const cnUsernameSupplementMinRunes = 8

var cnUsernameGapTerms = []string{
	"gongchandang",
	"xijinping",
}

type cnSupTerm struct {
	term string
	sk   string
	gap  bool
}

var (
	cnSupOnce sync.Once
	cnSupList []cnSupTerm
)

func cnUsernameSupplement(normalized, field string, regions []string) (Verdict, bool) {
	if !containsStr(regions, "cn") || !cnUsernameField(field) {
		return Verdict{}, false
	}
	folded := letterSkeleton(normalized)
	if folded == "" {
		return Verdict{}, false
	}
	var best cnSupTerm
	for _, term := range cnSupplementTerms() {
		if folded != term.sk && !strings.HasPrefix(folded, term.sk) && !strings.HasSuffix(folded, term.sk) {
			continue
		}
		if best.term == "" || utf8.RuneCountInString(term.sk) > utf8.RuneCountInString(best.sk) {
			best = term
		}
	}
	if best.term == "" {
		return Verdict{}, false
	}
	return Verdict{
		Outcome: OutcomeRegionalRestricted,
		RuleID:  "supplement/cn-username:" + best.term,
		Term:    best.term,
		Matched: "SKELETON_BOUNDARY",
		Source:  cnUsernameSupplementSource,
	}, true
}

func cnSupplementTerms() []cnSupTerm {
	cnSupOnce.Do(func() {
		seen := map[string]struct{}{}
		add := func(term string, gap bool) {
			sk := letterSkeleton(Normalize(term))
			if utf8.RuneCountInString(sk) < cnUsernameSupplementMinRunes {
				return
			}
			if _, ok := seen[sk]; ok {
				return
			}
			seen[sk] = struct{}{}
			cnSupList = append(cnSupList, cnSupTerm{term: term, sk: sk, gap: gap})
		}
		for _, rule := range currentEngine().rules {
			if !strings.Contains(rule.Source, "regions/cn/politics.toml") {
				continue
			}
			if !containsStr(rule.Regions, "cn") || !containsStr(rule.Fields, FieldUsername) {
				continue
			}
			add(rule.Normalized, false)
		}
		for _, term := range cnUsernameGapTerms {
			add(term, true)
		}
	})
	return cnSupList
}

func cnUsernameField(field string) bool {
	switch field {
	case FieldUsername, FieldSlug, FieldBotUsername, FieldDisplayName:
		return true
	default:
		return false
	}
}

// letterSkeleton is the confusable skeleton, lowercased again because UTS #39
// can map ASCII 0 to uppercase O before asciiFold, then with digits removed.
func letterSkeleton(normalized string) string {
	return stripDigits(strings.ToLower(Skeleton(normalized)))
}

func stripDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return -1
		}
		return r
	}, s)
}

// CnUsernameSupplementStats is the curated-term count and how many of those
// terms were absent from policy-2026110602.
func CnUsernameSupplementStats() (curated, gap int) {
	for _, term := range cnSupplementTerms() {
		if term.gap {
			gap++
			continue
		}
		curated++
	}
	return curated, gap
}
