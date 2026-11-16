package usernamepolicy

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The supplement folds the confusable skeleton, drops remaining digits,
// and then accepts an exact, prefix, or suffix hit. Upstream PREFIX and
// SUFFIX stop at a letter or digit, and CONFUSABLE compares the whole
// skeleton, so a lookalike glued to digits would otherwise pass.
// A term buried in the middle (notgongchandangx) is not a hit.
// Skeletons shorter than 8 are ignored, the same floor the upstream build
// uses before it will attach PREFIX/SUFFIX to a latin term.
//
// Terms come from the latin cn username rules already in
// policy-2026110602 and from the runtime MIT-file rules. There is no
// handwritten exception list. Global-only checks are unchanged.
// must-allow still has "username global xijinping". Entry points pass
// global and cn.
const cnUsernameSupplementSource = "teamgram-server/pkg/usernamepolicy/cn_username_supplement.go"

const cnUsernameSupplementMinRunes = 8

type cnSupTerm struct {
	term string
	sk   string
}

func (e *Engine) cnUsernameSupplement(normalized, field string, regions []string) (Verdict, bool) {
	if !containsStr(regions, "cn") || !cnUsernameField(field) {
		return Verdict{}, false
	}
	folded := letterSkeleton(normalized)
	if folded == "" {
		return Verdict{}, false
	}
	var best cnSupTerm
	for _, term := range e.cnSupplementTerms() {
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

func (e *Engine) cnSupplementTerms() []cnSupTerm {
	e.supOnce.Do(func() {
		seen := map[string]struct{}{}
		for _, rule := range e.rules {
			latin := strings.Contains(rule.Source, "regions/cn/politics.toml") || strings.Contains(rule.Source, runtimeLexiconSource)
			if !latin {
				continue
			}
			if !containsStr(rule.Regions, "cn") || !containsStr(rule.Fields, FieldUsername) {
				continue
			}
			sk := letterSkeleton(rule.Normalized)
			if utf8.RuneCountInString(sk) < cnUsernameSupplementMinRunes {
				continue
			}
			if _, ok := seen[sk]; ok {
				continue
			}
			seen[sk] = struct{}{}
			e.supList = append(e.supList, cnSupTerm{term: rule.Normalized, sk: sk})
		}
	})
	return e.supList
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

// CnUsernameSupplementStats is the number of latin cn username skeletons
// at least 8 runes long that the runtime supplement folds.
func CnUsernameSupplementStats() int {
	return len(runtimeEngine().cnSupplementTerms())
}
