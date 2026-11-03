package usernamepolicy

import "encoding/json"

// MarshalLexicon writes the appendix C lexicon as a policy envelope.
// Normalized and skeleton are filled by this package, so the JSON the
// server loads is the same bytes the tests compare.
func MarshalLexicon() ([]byte, error) {
	rules, err := buildRules()
	if err != nil {
		return nil, err
	}
	env := envelope{
		Name:        "policy",
		Version:     lexiconVersion,
		Schema:      1,
		GeneratedAt: "2026-11-03T00:00:00Z",
		GeneratedBy: "teamgram-server pkg/usernamepolicy (appendix C; tellomi/tellomi dist was not readable)",
		Inputs:      []any{},
	}
	env.Payload.Rules = rules
	env.Payload.Allow = []allowEntry{}
	return json.MarshalIndent(env, "", "  ")
}
