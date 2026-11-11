package langpack

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTellomiPlaceholderConsistency checks that every Tellomi client key
// keeps the same substitution tokens as the English string in that pack.
// A language that uses plural forms is compared form-by-form when English
// also has that form, or against the English value otherwise.
func TestTellomiPlaceholderConsistency(t *testing.T) {
	doc := loadTellomiManifest(t)
	checks := []struct {
		platform string
		keys     []string
	}{
		{"android", doc.Android.all()},
		{"ios", doc.IOS.all()},
	}
	for _, tc := range checks {
		dir := filepath.Join("..", "..", "data", "langpack", tc.platform)
		en := readPackStrings(t, filepath.Join(dir, "en.json.gz"))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, ent := range entries {
			name := ent.Name()
			if ent.IsDir() || len(name) < len(".json.gz") || name[len(name)-len(".json.gz"):] != ".json.gz" {
				continue
			}
			code := name[:len(name)-len(".json.gz")]
			if code == "en" {
				continue
			}
			pack := readPackStrings(t, filepath.Join(dir, name))
			for _, key := range tc.keys {
				src, ok := en[key]
				if !ok {
					t.Errorf("%s en missing %s", tc.platform, key)
					continue
				}
				got, ok := pack[key]
				if !ok {
					t.Errorf("%s %s missing %s", tc.platform, code, key)
					continue
				}
				for _, form := range packForms {
					want := src.forms[form]
					text := got.forms[form]
					if text == "" {
						continue
					}
					if want == "" {
						want = src.forms["value"]
					}
					if want == "" {
						want = src.forms["other"]
					}
					if want == "" {
						continue
					}
					if !samePlaceholderTokens(want, text) {
						t.Errorf("%s %s %s %s placeholders %q -> %q (%q vs %q)", tc.platform, code, key, form, placeholderTokens(want), placeholderTokens(text), want, text)
					}
				}
			}
		}
	}
}
