package usernamepolicy

import (
	"bytes"
	"os"
	"testing"
)

func TestOfficialLexicon(t *testing.T) {
	e := officialEngine()
	if e.Version() != 2026092301 {
		t.Fatalf("version %d", e.Version())
	}
	if e.RuleCount() != 470 {
		t.Fatalf("rules %d", e.RuleCount())
	}
}

func TestOverlayFileMatchesSpec(t *testing.T) {
	want, err := MarshalOverlay()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(overlayPolicy, want) {
		if os.Getenv("TELLOMI_WRITE_POLICY") == "1" {
			if err := os.WriteFile("policy/overlay-d56.json", want, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Fatal("rewrote policy/overlay-d56.json; run the tests again")
		}
		t.Fatalf("overlay drifted (%d embedded, %d spec)", len(overlayPolicy), len(want))
	}
	extra, err := Load(want)
	if err != nil {
		t.Fatal(err)
	}
	base := officialEngine()
	have := map[string]struct{}{}
	for _, r := range base.rules {
		have[r.Term] = struct{}{}
	}
	for _, r := range extra.rules {
		if r.Source != "requirements-v2/appendix-c" {
			t.Fatalf("source %s", r.Source)
		}
		// Affix supplements for names the official file already reserves exactly.
		if _, ok := have[r.Term]; ok && containsStr(r.Matches, MatchNormalizedExact) {
			t.Fatalf("overlay redefines official term %s", r.Term)
		}
	}
}
