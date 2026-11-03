package usernamepolicy

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	schemaMin = 1
	schemaMax = 1

	OutcomeAllowed            = "allowed"
	OutcomeReserved           = "reserved"
	OutcomeBrandProtected     = "brand_protected"
	OutcomeImpersonation      = "impersonation"
	OutcomeSecuritySensitive  = "security_sensitive"
	OutcomeRegionalRestricted = "regional_restricted"
	OutcomeContentRestricted  = "content_restricted"
	OutcomeInvalidUnicode     = "invalid_unicode"
	OutcomeConfusable         = "confusable"

	MatchExact           = "EXACT"
	MatchNormalizedExact = "NORMALIZED_EXACT"
	MatchPrefix          = "PREFIX"
	MatchSuffix          = "SUFFIX"
	MatchContains        = "CONTAINS"
	MatchConfusable      = "CONFUSABLE"

	FieldUsername         = "username"
	FieldSlug             = "slug"
	FieldDisplayName      = "display_name"
	FieldProfileName      = "profile_name"
	FieldGroupName        = "group_name"
	FieldGroupDescription = "group_description"
	FieldChannelName      = "channel_name"
	FieldBotUsername      = "bot_username"
	FieldBio              = "bio"
)

// Verdict is the engine result. RuleID is for logs. Callers that answer a
// client must not copy it into the response.
type Verdict struct {
	Outcome string
	RuleID  string
	Term    string
	Matched string
	Source  string
}

// IsAllowed reports a name that no rule refused.
func (v Verdict) IsAllowed() bool { return v.Outcome == OutcomeAllowed || v.Outcome == "" }

type rule struct {
	ID         string   `json:"id"`
	Term       string   `json:"term"`
	Normalized string   `json:"normalized"`
	Skeleton   string   `json:"skeleton"`
	Matches    []string `json:"matches"`
	Fields     []string `json:"fields"`
	Regions    []string `json:"regions"`
	Outcome    string   `json:"outcome"`
	Source     string   `json:"source"`
}

type allowEntry struct {
	Normalized string   `json:"normalized"`
	Fields     []string `json:"fields"`
	Regions    []string `json:"regions"`
	Note       string   `json:"note,omitempty"`
}

type lexicon struct {
	rules []rule
	allow []allowEntry
}

// Engine is an immutable lexicon index.
type Engine struct {
	version    uint64
	rules      []rule
	allow      []allowEntry
	byRaw      map[string][]int
	byNorm     map[string][]int
	bySkeleton map[string][]int
	byAffix    map[string][]int
	contains   []int
}

// Version is the lexicon version from the envelope.
func (e *Engine) Version() uint64 { return e.version }

// RuleCount is the number of rules that loaded.
func (e *Engine) RuleCount() int {
	if e == nil {
		return 0
	}
	return len(e.rules)
}

type envelope struct {
	Name        string `json:"name"`
	Version     uint64 `json:"version"`
	Schema      uint32 `json:"schema"`
	GeneratedAt string `json:"generated_at,omitempty"`
	GeneratedBy string `json:"generated_by,omitempty"`
	Inputs      []any  `json:"inputs"`
	Payload     struct {
		Rules []rule       `json:"rules"`
		Allow []allowEntry `json:"allow"`
	} `json:"payload"`
}

// Load parses a policy envelope and indexes it. A bad file is an error,
// not a partial engine.
func Load(b []byte) (*Engine, error) {
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("usernamepolicy: parse: %w", err)
	}
	if env.Name != "policy" {
		return nil, fmt.Errorf("usernamepolicy: name %q", env.Name)
	}
	if env.Schema < schemaMin || env.Schema > schemaMax {
		return nil, fmt.Errorf("usernamepolicy: schema %d", env.Schema)
	}
	if err := validate(env.Payload.Rules); err != nil {
		return nil, err
	}
	return index(env.Version, env.Payload.Rules, env.Payload.Allow), nil
}

func validate(rules []rule) error {
	ids := map[string]struct{}{}
	for _, r := range rules {
		if _, ok := ids[r.ID]; ok {
			return fmt.Errorf("usernamepolicy: duplicate rule id %s", r.ID)
		}
		ids[r.ID] = struct{}{}
		if r.Term == "" || r.Normalized == "" {
			return fmt.Errorf("usernamepolicy: rule %s has an empty term", r.ID)
		}
		if len(r.Matches) == 0 || len(r.Fields) == 0 || len(r.Regions) == 0 {
			return fmt.Errorf("usernamepolicy: rule %s matches nothing", r.ID)
		}
		for _, m := range r.Matches {
			if !knownMatch(m) {
				return fmt.Errorf("usernamepolicy: rule %s bad match %s", r.ID, m)
			}
		}
		if containsStr(r.Matches, MatchConfusable) && r.Skeleton == "" {
			return fmt.Errorf("usernamepolicy: rule %s is CONFUSABLE without a skeleton", r.ID)
		}
		if containsStr(r.Matches, MatchContains) && !longEnoughForContains(r.Normalized) {
			return fmt.Errorf("usernamepolicy: rule %s CONTAINS term too short", r.ID)
		}
		if r.Outcome == OutcomeAllowed {
			return fmt.Errorf("usernamepolicy: rule %s outcome=allowed; use the allowlist", r.ID)
		}
		if !knownOutcome(r.Outcome) {
			return fmt.Errorf("usernamepolicy: rule %s bad outcome %s", r.ID, r.Outcome)
		}
	}
	return nil
}

func knownMatch(m string) bool {
	switch m {
	case MatchExact, MatchNormalizedExact, MatchPrefix, MatchSuffix, MatchContains, MatchConfusable:
		return true
	default:
		return false
	}
}

func knownOutcome(o string) bool {
	switch o {
	case OutcomeReserved, OutcomeBrandProtected, OutcomeImpersonation, OutcomeSecuritySensitive,
		OutcomeRegionalRestricted, OutcomeContentRestricted, OutcomeInvalidUnicode, OutcomeConfusable:
		return true
	default:
		return false
	}
}

func longEnoughForContains(normalized string) bool {
	cjk := 0
	for _, c := range normalized {
		if isCJK(c) {
			cjk++
		}
	}
	if cjk > 0 {
		return cjk >= 2
	}
	return utf8.RuneCountInString(normalized) >= 5
}

func isCJK(c rune) bool {
	return (c >= 0x3400 && c <= 0x4DBF) || (c >= 0x4E00 && c <= 0x9FFF) || (c >= 0xF900 && c <= 0xFAFF) || (c >= 0x20000 && c <= 0x2FA1F)
}

func index(version uint64, rules []rule, allow []allowEntry) *Engine {
	e := &Engine{
		version:    version,
		rules:      rules,
		allow:      allow,
		byRaw:      map[string][]int{},
		byNorm:     map[string][]int{},
		bySkeleton: map[string][]int{},
		byAffix:    map[string][]int{},
	}
	for i, r := range rules {
		for _, m := range r.Matches {
			switch m {
			case MatchExact:
				e.byRaw[r.Term] = append(e.byRaw[r.Term], i)
			case MatchNormalizedExact:
				e.byNorm[r.Normalized] = append(e.byNorm[r.Normalized], i)
			case MatchConfusable:
				e.bySkeleton[r.Skeleton] = append(e.bySkeleton[r.Skeleton], i)
			case MatchPrefix, MatchSuffix:
				if !containsInt(e.byAffix[r.Normalized], i) {
					e.byAffix[r.Normalized] = append(e.byAffix[r.Normalized], i)
				}
			case MatchContains:
				e.contains = append(e.contains, i)
			}
		}
	}
	return e
}

// Check reports whether input may be used for field in any of regions.
// regions empty means global. Several rules can match; the longest term wins.
func (e *Engine) Check(input string, field string, regions []string) Verdict {
	if e == nil || input == "" {
		return Verdict{Outcome: OutcomeAllowed}
	}
	if len(regions) == 0 {
		regions = []string{"global"}
	}
	if HasDisallowedControl(input) {
		return Verdict{Outcome: OutcomeInvalidUnicode}
	}
	normalized := Normalize(input)
	if normalized == "" {
		return Verdict{Outcome: OutcomeInvalidUnicode}
	}
	if !asciiOnlyField(field) && MixedScript(normalized) {
		return Verdict{Outcome: OutcomeConfusable}
	}
	if e.allowed(normalized, field, regions) {
		return Verdict{Outcome: OutcomeAllowed}
	}

	var best int = -1
	var bestMatch string
	consider := func(idx int, matched string) {
		if idx < 0 {
			return
		}
		if best < 0 || utf8.RuneCountInString(e.rules[idx].Normalized) > utf8.RuneCountInString(e.rules[best].Normalized) {
			best = idx
			bestMatch = matched
		}
	}
	if idx, kind, ok := e.matchEquality(input, normalized, field, regions); ok {
		consider(idx, kind)
	}
	if idx, matched, ok := e.matchAffix(normalized, field, regions); ok {
		consider(idx, matched)
	}
	if idx, ok := e.first(e.bySkeleton[Skeleton(normalized)], field, regions, MatchConfusable); ok {
		consider(idx, MatchConfusable)
	}
	if idx, ok := e.matchContains(normalized, field, regions); ok {
		consider(idx, MatchContains)
	}
	if best < 0 {
		return Verdict{Outcome: OutcomeAllowed}
	}
	r := e.rules[best]
	return Verdict{
		Outcome: r.Outcome,
		RuleID:  r.ID,
		Term:    r.Term,
		Matched: bestMatch,
		Source:  r.Source,
	}
}

func (e *Engine) matchEquality(raw, normalized, field string, regions []string) (int, string, bool) {
	if idx, ok := e.first(e.byRaw[raw], field, regions, MatchExact); ok {
		return idx, MatchExact, true
	}
	idx, ok := e.first(e.byNorm[normalized], field, regions, MatchNormalizedExact)
	return idx, MatchNormalizedExact, ok
}

// Prefix and suffix need a non-alphanumeric boundary, so administer does
// not match admin and admin01 does not match admin. admin_01 does.
// A prefix hit is returned before any suffix hit, matching the engine
// that the corpus was written against.
func (e *Engine) matchAffix(normalized, field string, regions []string) (int, string, bool) {
	chars := []rune(normalized)
	for end := 1; end < len(chars); end++ {
		if isAlnum(chars[end]) {
			continue
		}
		candidate := string(chars[:end])
		if idx, ok := e.first(e.byAffix[candidate], field, regions, MatchPrefix); ok {
			return idx, MatchPrefix, true
		}
	}
	for start := 1; start < len(chars); start++ {
		if isAlnum(chars[start-1]) {
			continue
		}
		candidate := string(chars[start:])
		if idx, ok := e.first(e.byAffix[candidate], field, regions, MatchSuffix); ok {
			return idx, MatchSuffix, true
		}
	}
	return 0, "", false
}

func (e *Engine) matchContains(normalized, field string, regions []string) (int, bool) {
	bestAt := -1
	bestIdx := -1
	for _, idx := range e.contains {
		r := e.rules[idx]
		if !r.applies(field, regions) {
			continue
		}
		at := strings.Index(normalized, r.Normalized)
		if at < 0 {
			continue
		}
		if bestIdx < 0 || at < bestAt || (at == bestAt && idx < bestIdx) {
			bestAt = at
			bestIdx = idx
		}
	}
	if bestIdx < 0 {
		return 0, false
	}
	return bestIdx, true
}

func (e *Engine) first(cands []int, field string, regions []string, want string) (int, bool) {
	for _, i := range cands {
		r := e.rules[i]
		if r.applies(field, regions) && containsStr(r.Matches, want) {
			return i, true
		}
	}
	return 0, false
}

func (e *Engine) allowed(normalized, field string, regions []string) bool {
	for _, a := range e.allow {
		if a.Normalized != normalized || !containsStr(a.Fields, field) {
			continue
		}
		for _, r := range a.Regions {
			if containsStr(regions, r) {
				return true
			}
		}
	}
	return false
}

func (r rule) applies(field string, regions []string) bool {
	if !containsStr(r.Fields, field) {
		return false
	}
	for _, reg := range r.Regions {
		if containsStr(regions, reg) {
			return true
		}
	}
	return false
}

func asciiOnlyField(field string) bool {
	return field == FieldUsername || field == FieldSlug
}

func isAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func containsInt(ns []int, want int) bool {
	for _, n := range ns {
		if n == want {
			return true
		}
	}
	return false
}
