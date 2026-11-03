package usernamepolicy

import (
	"bytes"
	"os"
	"testing"
)

func TestEmbeddedMatchesSpec(t *testing.T) {
	want, err := MarshalLexicon()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embeddedPolicy, want) {
		if os.Getenv("TELLOMI_WRITE_POLICY") == "1" {
			if err := os.WriteFile("policy/policy-2026110301.json", want, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Fatal("rewrote policy/policy-2026110301.json; run the tests again")
		}
		t.Fatalf("embedded policy drifted from spec (%d bytes embedded, %d from spec). Set TELLOMI_WRITE_POLICY=1 to rewrite.", len(embeddedPolicy), len(want))
	}
	e, err := Load(want)
	if err != nil {
		t.Fatal(err)
	}
	if e.Version() != lexiconVersion {
		t.Fatalf("version %d", e.Version())
	}
	if e.RuleCount() < 200 {
		t.Fatalf("rule count %d", e.RuleCount())
	}
}
