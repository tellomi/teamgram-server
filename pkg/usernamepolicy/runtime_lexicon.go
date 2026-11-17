package usernamepolicy

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy/lexiconsrc"
)

const runtimeLexiconSource = "lexiconsrc/konsheng"

// runtimeRules are the MIT category lines that policy-2026110602 left
// unpublished because they also occur as whole lines of the excluded
// Tencent file, plus the reviewed GFW allowlist. The bytes loaded here
// are the vendored konsheng files and generated/gfw-selected.tsv. The Tencent
// file and the rest of the GFW file are not inputs. Editorial drops stay out.
func runtimeRules() ([]rule, error) {
	terms, err := lexiconsrc.PromotedTerms()
	if err != nil {
		return nil, err
	}
	extra, err := lexiconsrc.SupplementTerms()
	if err != nil {
		return nil, err
	}
	terms = append(terms, extra...)
	exact, err := lexiconsrc.RuntimeExact()
	if err != nil {
		return nil, err
	}
	pinyin, err := lexiconsrc.RuntimePinyin()
	if err != nil {
		return nil, err
	}
	var rules []rule
	seen := map[string]struct{}{}
	add := func(id, term, source string, spec lexiconsrc.TermSpec) error {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("usernamepolicy: duplicate runtime rule %s", id)
		}
		seen[id] = struct{}{}
		normalized := Normalize(term)
		if normalized == "" {
			return fmt.Errorf("usernamepolicy: runtime term %s normalizes empty", term)
		}
		skeleton := ""
		if containsStr(spec.Matches, MatchConfusable) {
			skeleton = Skeleton(normalized)
		}
		rules = append(rules, rule{
			ID:         id,
			Term:       term,
			Normalized: normalized,
			Skeleton:   skeleton,
			Matches:    append([]string{}, spec.Matches...),
			Fields:     append([]string{}, spec.Fields...),
			Regions:    []string{"cn"},
			Outcome:    OutcomeRegionalRestricted,
			Source:     source,
		})
		return nil
	}
	for _, term := range terms {
		spec := lexiconsrc.SpecFor(term.Term, false)
		if _, ok := exact[term.Term]; ok || lexiconsrc.FragmentExact(term.Term) {
			spec.Matches = []string{MatchNormalizedExact}
		}
		source := runtimeLexiconSource + " " + strings.Join(term.Files, ",")
		if err := add("lexiconsrc/runtime:"+term.Term, term.Term, source, spec); err != nil {
			return nil, err
		}
	}
	for word, py := range pinyin {
		origin := ""
		for _, term := range terms {
			if term.Term == word {
				origin = strings.Join(term.Files, ",")
				break
			}
		}
		if origin == "" {
			return nil, fmt.Errorf("usernamepolicy: pinyin %s lost its source", word)
		}
		spec := lexiconsrc.SpecFor(py, true)
		if utf8.RuneCountInString(py) < 8 {
			return nil, fmt.Errorf("usernamepolicy: short runtime pinyin %s", py)
		}
		source := fmt.Sprintf("%s pypinyin 0.55.0 of %s from %s", runtimeLexiconSource, word, origin)
		if err := add("lexiconsrc/runtime-pinyin:"+py, py, source, spec); err != nil {
			return nil, err
		}
	}
	if err := validate(rules); err != nil {
		return nil, err
	}
	return rules, nil
}
