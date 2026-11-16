package usernamepolicy

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// cnUsernameSupplementTerms are the bare cn username terms missing from
// tellomi/tellomi policy/lexicons/regions/cn/politics.toml at the commit
// that produced policy-2026110602 (politics.toml header cites
// konsheng/Sensitive-lexicon@d967c30 and fwwdn/sensitive-stop-words@a7d06bb).
// Those lists were already curated into the toml. This file does not import
// them again. The two terms below are the ones the product requirement adds,
// in the same region and fields as the other latin politics rules.
//
// Upstream match modes do not express this shape. PREFIX/SUFFIX stop at a
// letter or digit, and CONFUSABLE compares the whole skeleton, so
// g0ngchandang88 is neither. Skeleton-contains after the ASCII fold and
// after dropping remaining digits covers case, NFKC, underscore, digit, and
// lookalike bypasses for these long terms only. Terms shorter than 8
// skeletons are ignored. policy-2026110602.json is left byte-for-byte.
//
// When politics.toml grows a rebuild, add the same [[rule]] blocks there.
// Keep this supplement until that rebuild has a match mode that covers the
// digit-plus-lookalike form. Global-only checks stay allowed: must-allow
// still says "username global xijinping". Entry points pass global and cn,
// so cn is what rejects them.
const cnUsernameSupplementSource = "teamgram-server/pkg/usernamepolicy/cn_username_supplement.go"

const cnUsernameSupplementMinRunes = 8

var cnUsernameSupplementTerms = []string{
	"gongchandang",
	"xijinping",
}

func cnUsernameSupplement(normalized, field string, regions []string) (Verdict, bool) {
	if !containsStr(regions, "cn") || !cnUsernameField(field) {
		return Verdict{}, false
	}
	// UTS #39 can map ASCII 0 to uppercase O before asciiFold, which only
	// folds the digit itself. Lowercase again so the skeletons compare.
	folded := strings.ToLower(stripDigits(Skeleton(normalized)))
	if folded == "" {
		return Verdict{}, false
	}
	var best string
	var bestSk string
	for _, term := range cnUsernameSupplementTerms {
		sk := strings.ToLower(stripDigits(Skeleton(Normalize(term))))
		if utf8.RuneCountInString(sk) < cnUsernameSupplementMinRunes {
			continue
		}
		if !strings.Contains(folded, sk) {
			continue
		}
		if best == "" || utf8.RuneCountInString(sk) > utf8.RuneCountInString(bestSk) {
			best = term
			bestSk = sk
		}
	}
	if best == "" {
		return Verdict{}, false
	}
	return Verdict{
		Outcome: OutcomeRegionalRestricted,
		RuleID:  "supplement/cn-username:" + best,
		Term:    best,
		Matched: "SKELETON_CONTAINS",
		Source:  cnUsernameSupplementSource,
	}, true
}

func cnUsernameField(field string) bool {
	switch field {
	case FieldUsername, FieldSlug, FieldBotUsername, FieldDisplayName:
		return true
	default:
		return false
	}
}

func stripDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return -1
		}
		return r
	}, s)
}
