package usernamepolicy

import "encoding/json"

// MarshalOverlay writes the D-56 supplement. It is a separate envelope.
// policy-2026092301.json is not rewritten.
func MarshalOverlay() ([]byte, error) {
	rules, err := buildRules(overlaySpecs())
	if err != nil {
		return nil, err
	}
	env := envelope{
		Name:        "policy",
		Version:     2026110302,
		Schema:      1,
		GeneratedAt: "2026-11-03T00:00:00Z",
		GeneratedBy: "teamgram-server pkg/usernamepolicy overlay for requirements v2 appendix C / D-56; not part of policy-2026092301",
		Inputs:      []any{},
	}
	env.Payload.Rules = rules
	env.Payload.Allow = []allowEntry{}
	return json.MarshalIndent(env, "", "  ")
}

func buildRules(specs []termSpec) ([]rule, error) {
	var rules []rule
	seen := map[string]struct{}{}
	for _, spec := range specs {
		for _, word := range spec.words {
			id := spec.prefix + ":" + word
			if _, ok := seen[id]; ok {
				return nil, errDuplicate(id)
			}
			seen[id] = struct{}{}
			normalized := Normalize(word)
			skeleton := ""
			if containsStr(spec.matches, MatchConfusable) {
				skeleton = Skeleton(normalized)
			}
			rules = append(rules, rule{
				ID:         id,
				Term:       word,
				Normalized: normalized,
				Skeleton:   skeleton,
				Matches:    append([]string{}, spec.matches...),
				Fields:     append([]string{}, spec.fields...),
				Regions:    []string{"global"},
				Outcome:    spec.outcome,
				Source:     spec.source,
			})
		}
	}
	return rules, nil
}

type dupError string

func (e dupError) Error() string { return string(e) }

func errDuplicate(id string) error { return dupError("usernamepolicy: spec " + id) }
