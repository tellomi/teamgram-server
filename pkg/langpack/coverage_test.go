package langpack

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// langpackPlaceholder is the client substitution tokens langpack_fill keeps
// intact: %@, %1$s, %1$@, {count}, {}, and a bare %2.
var langpackPlaceholder = regexp.MustCompile(
	`%%|%(?:\d+\$)?[#0\- +']*(?:\d+|\*)?(?:\.\d+)?(?:hh|h|ll|l|z|j|t|q|L)?[diuoxXfFeEgGaAcspn@]|\{[A-Za-z_][A-Za-z0-9_]*\}|\{\}|\{[0-9]+\}|%\d+\b`,
)

var langpackLinkTarget = regexp.MustCompile(`\]\(([^)\n]*)\)`)

func TestKeyCountWithinOnePercentOfEnglish(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	for _, platform := range []string{"ios", "android", "tdesktop", "macos"} {
		enDiff, err := cat.Difference(platform, "en", 0)
		if err != nil {
			t.Fatal(platform, err)
		}
		enKeys := map[string]struct{}{}
		for _, s := range enDiff.GetStrings() {
			enKeys[s.GetKey()] = struct{}{}
		}
		enN := len(enKeys)
		if enN < 1000 {
			t.Fatalf("%s en keys %d", platform, enN)
		}
		for _, lang := range OfficialLanguages {
			if lang.Code == "en" {
				continue
			}
			diff, err := cat.Difference(platform, lang.Code, 0)
			if err != nil {
				t.Fatal(platform, lang.Code, err)
			}
			have := map[string]struct{}{}
			for _, s := range diff.GetStrings() {
				have[s.GetKey()] = struct{}{}
			}
			missing := 0
			for key := range enKeys {
				if _, ok := have[key]; !ok {
					missing++
				}
			}
			if missing*100 > enN {
				t.Errorf("%s %s missing %d of %d English keys (%.2f%%)", platform, lang.Code, missing, enN, 100*float64(missing)/float64(enN))
			}
		}
	}
}

func TestMachineTranslationsKeepPlaceholders(t *testing.T) {
	root := filepath.Join("..", "..", "data", "langpack")
	for _, platform := range []string{"ios", "android", "tdesktop", "macos"} {
		en := readPackStrings(t, filepath.Join(root, platform, "en.json.gz"))
		hans := readPackStrings(t, filepath.Join(root, platform, "zh-hans.json.gz"))
		hant := readPackStrings(t, filepath.Join(root, platform, "zh-hant.json.gz"))
		dir := filepath.Join(root, platform)
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
			sibling := hans
			if code == "zh-hans" {
				sibling = hant
			}
			for key, item := range pack {
				if !item.MT && len(item.MTForms) == 0 {
					continue
				}
				srcPack := en
				if item.Via == "opencc" {
					srcPack = sibling
				}
				src, ok := srcPack[key]
				if !ok {
					t.Errorf("%s %s %s has no source", platform, code, key)
					continue
				}
				for _, form := range packForms {
					if !item.MT && !item.MTForms[form] {
						continue
					}
					got := item.forms[form]
					want := src.forms[form]
					if got == "" && want == "" {
						continue
					}
					if !samePlaceholderTokens(want, got) {
						t.Errorf("%s %s %s %s placeholders %q -> %q", platform, code, key, form, placeholderTokens(want), placeholderTokens(got))
					}
				}
			}
		}
	}
}

var packForms = []string{"value", "zero", "one", "two", "few", "many", "other"}

type packItem struct {
	MT      bool
	Via     string
	MTForms map[string]bool
	forms   map[string]string
}

func readPackStrings(t *testing.T, path string) map[string]packItem {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var doc struct {
		Strings []map[string]any `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(path, err)
	}
	out := make(map[string]packItem, len(doc.Strings))
	for _, raw := range doc.Strings {
		key, _ := raw["key"].(string)
		item := packItem{forms: map[string]string{}}
		if mt, ok := raw["mt"].(bool); ok {
			item.MT = mt
		}
		if via, ok := raw["mt_via"].(string); ok {
			item.Via = via
		}
		item.MTForms = map[string]bool{}
		if listed, ok := raw["mt_forms"].([]any); ok {
			for _, form := range listed {
				if name, ok := form.(string); ok {
					item.MTForms[name] = true
				}
			}
		}
		for _, form := range packForms {
			if text, ok := raw[form].(string); ok {
				item.forms[form] = text
			}
		}
		out[key] = item
	}
	return out
}

func placeholderTokens(text string) []string {
	return append(langpackPlaceholder.FindAllString(text, -1), langpackLinkTarget.FindAllString(text, -1)...)
}

func samePlaceholderTokens(src, dst string) bool {
	a := placeholderTokens(src)
	b := placeholderTokens(dst)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
