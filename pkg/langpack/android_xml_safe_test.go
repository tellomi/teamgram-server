package langpack

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestAndroidPacksSurviveStockXMLEscape writes every shipped Android pack the
// way LocaleController.saveRemoteLocaleStrings does and reads it back with a
// strict XML parser. The number of entries the server drops is the gap
// between the file and the served pack. After the SearchEngine URL strings
// and the broken Greek SentAppCode are repaired, that gap is zero.
func TestAndroidPacksSurviveStockXMLEscape(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	dir := filepath.Join("..", "..", "data", "langpack", "android")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var packs int
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".json.gz") {
			continue
		}
		packs++
		code := strings.TrimSuffix(name, ".json.gz")
		raw := readPackStrings(t, filepath.Join(dir, name))
		if cat.AndroidXMLDrops(code) != 0 {
			t.Errorf("%s dropped %d strings", code, cat.AndroidXMLDrops(code))
		}
		diff, err := cat.Difference("android", code, 0)
		if err != nil {
			t.Fatal(code, err)
		}
		if diff.GetFromVersion() != 0 {
			t.Fatalf("%s full pack from_version %d", code, diff.GetFromVersion())
		}
		served := map[string]string{}
		for _, s := range diff.GetStrings() {
			if s.GetKey() == "" {
				t.Fatalf("%s empty key", code)
			}
			if !androidKeyRE.MatchString(s.GetKey()) {
				t.Fatalf("%s served key %q", code, s.GetKey())
			}
			switch s.GetPredicateName() {
			case "langPackString":
				served[s.GetKey()] = s.GetValue()
			case "langPackStringPluralized":
				for _, pair := range []struct {
					suf string
					val string
				}{
					{"_zero", s.GetZeroValue().GetValue()},
					{"_one", s.GetOneValue().GetValue()},
					{"_two", s.GetTwoValue().GetValue()},
					{"_few", s.GetFewValue().GetValue()},
					{"_many", s.GetManyValue().GetValue()},
					{"_other", s.GetOtherValue()},
				} {
					if pair.val == "" {
						continue
					}
					served[s.GetKey()+pair.suf] = pair.val
				}
			default:
				t.Fatalf("%s %s predicate %s", code, s.GetKey(), s.GetPredicateName())
			}
			for _, part := range stringParts(s) {
				if strings.Contains(part, "client=chrome&q=") || strings.Contains(part, "&nresults=") || strings.Contains(part, "&command=") {
					t.Fatalf("%s still serves %s %q", code, s.GetKey(), part)
				}
			}
		}
		if len(diff.GetStrings()) != len(raw) {
			t.Errorf("%s served %d entries, file has %d (dropped %d)", code, len(diff.GetStrings()), len(raw), len(raw)-len(diff.GetStrings()))
		}
		if err := parseAndroidLocaleXML(served); err != nil {
			t.Errorf("%s xml: %v", code, err)
		}
	}
	if packs != 32 {
		t.Fatalf("android packs %d, want 32", packs)
	}
}

// TestAndroidOldVersionReplacedWithFullPack is the recovery for a phone that
// already saved a broken locale file. Its cached version is below the fix,
// so getDifference returns the whole pack with from_version 0. A client that
// already has the fix version gets an empty difference.
func TestAndroidOldVersionReplacedWithFullPack(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	const cached = int32(61570955) // zh-hans version that shipped the broken URLs
	// Production 35aed0c stored this version even when the XML parse failed.
	// An equal getDifference is empty, so the cutover has to sit strictly above it.
	const production = int32(61638734)
	fix := cat.AndroidXMLFixVersion("android", "zh-hans")
	ver, ok := cat.Version("android", "zh-hans")
	if !ok || fix <= production || fix <= cached || ver < fix {
		t.Fatalf("zh-hans version %d fix %d ok %v", ver, fix, ok)
	}
	full, err := cat.Difference("android", "zh-hans", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, from := range []int32{cached, production} {
		got, err := cat.Difference("android", "zh-hans", from)
		if err != nil {
			t.Fatal(err)
		}
		if got.GetLangCode() != "zh-hans" || got.GetFromVersion() != 0 || got.GetVersion() != ver || len(got.GetStrings()) != len(full.GetStrings()) {
			t.Fatalf("from %d diff lang=%s from_version=%d version=%d strings=%d full=%d", from, got.GetLangCode(), got.GetFromVersion(), got.GetVersion(), len(got.GetStrings()), len(full.GetStrings()))
		}
	}
	got, err := cat.Difference("android", "zh-hans", cached)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetStrings()) < 10000 {
		t.Fatalf("full pack strings %d", len(got.GetStrings()))
	}
	empty, err := cat.Difference("android", "zh-hans", fix)
	if err != nil || len(empty.GetStrings()) != 0 || empty.GetFromVersion() != fix || empty.GetVersion() != ver {
		t.Fatalf("at fix version %+v %v", empty, err)
	}
	// iOS does not write XML and must keep the incremental from_version.
	ios, err := cat.Difference("ios", "zh-hans", 22454343)
	if err != nil || ios.GetFromVersion() != 22454343 {
		t.Fatalf("ios from_version %d %v", ios.GetFromVersion(), err)
	}
}

// TestAndroidLanguageMetadataAndCoreKeys is the contract stock
// LocaleController reads before it saves a pack. lang_code matches the
// short name the client derived from getLanguages. plural_code stays zh so
// the plural table matches. The new official keys and the product strings
// the export does not ship are present, and the login mail is not English.
func TestAndroidLanguageMetadataAndCoreKeys(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	lang, err := cat.Language("android", "zh-hans")
	if err != nil {
		t.Fatal(err)
	}
	if lang.GetLangCode() != "zh-hans" || lang.GetPluralCode() != "zh" || lang.GetBaseLangCode().GetValue() != "en" || !lang.GetOfficial() {
		t.Fatalf("language %+v", lang)
	}
	keys := []string{"Settings", "CompactMode", "TopicsSidebar", "AreTyping", "Reactions_one", "TellomiMapChooserPoint", "PrivacyPolicyUrl"}
	got, err := cat.Strings("android", "zh-hans", keys)
	if err != nil {
		t.Fatal(err)
	}
	en, err := cat.Strings("android", "en", []string{"Settings", "AreTyping"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Settings":               "设置",
		"CompactMode":            "紧凑模式",
		"TopicsSidebar":          "侧边栏",
		"AreTyping":              "正在输入…",
		"Reactions_one":          "%d 个回应",
		"TellomiMapChooserPoint": "打开地图",
		"PrivacyPolicyUrl":       "https://tellomi.cn/privacy",
	}
	for i, key := range keys {
		if got[i].GetKey() != key || got[i].GetValue() != want[key] {
			t.Fatalf("%s got %q (%s) want %q", key, got[i].GetValue(), got[i].GetPredicateName(), want[key])
		}
	}
	if got[0].GetValue() == en[0].GetValue() {
		t.Fatal("Settings is still English")
	}
	mail, err := cat.Strings("ios", "zh-hans", []string{"Login.EmailCodeBody", "Login.InvalidPhoneEmailBody", "Appstore.Fast.Chat6", "AppUpgrade.Running"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range mail {
		if s.GetPredicateName() != "langPackString" || s.GetValue() == "" {
			t.Fatalf("missing %s %+v", s.GetKey(), s)
		}
		low := strings.ToLower(s.GetValue())
		if strings.Contains(low, "i'm trying") || strings.Contains(low, "little sister") || strings.Contains(low, "optimizing tellomi") {
			t.Fatalf("%s still English %q", s.GetKey(), s.GetValue())
		}
	}
	if !strings.Contains(mail[1].GetValue(), "MNC") || !strings.Contains(mail[1].GetValue(), "%1$@") {
		t.Fatalf("login mail placeholders %q", mail[1].GetValue())
	}
}

func TestAndroidXMLGuardDropsUnsafeStrings(t *testing.T) {
	cat := NewCatalog(map[string]map[string]*Language{
		"android": {
			"en": {
				Code: "en", Version: 5, AndroidXMLFixVersion: 5,
				Strings: []Entry{
					{Key: "Ok", Value: "Privacy & Security", Version: 1},
					{Key: "Nbsp", Value: "a&#160;b", Version: 1},
					{Key: "Emoji", Value: "&#128197; Reminder", Version: 1},
					{Key: "Bold", Value: `<![CDATA[<b>]]>Tellomi<![CDATA[</b>]]>`, Version: 1},
					{Key: "BadURL", Value: "https://suggestqueries.google.com/complete/search?client=chrome&q=", Version: 2},
					{Key: "BadCDATA", Value: "<! [CDATA [<b>]]>Tellomi", Version: 2},
					{Key: "BadCtrl", Value: "a\x01b", Version: 2},
					{Key: "bad key", Value: "x", Version: 2},
					{Key: "BadPlural", Version: 2, Plural: &Plural{One: "one", Other: "a&b"}},
				},
			},
		},
	})
	if cat.AndroidXMLDrops("en") != 5 {
		t.Fatalf("drops %d", cat.AndroidXMLDrops("en"))
	}
	diff, err := cat.Difference("android", "en", 0)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range diff.GetStrings() {
		keys = append(keys, s.GetKey())
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "Bold,Emoji,Nbsp,Ok" {
		t.Fatalf("kept %v", keys)
	}
	if err := parseAndroidLocaleXML(map[string]string{
		"Ok":    "Privacy & Security",
		"Nbsp":  "a&#160;b",
		"Emoji": "&#128197; Reminder",
		"Bold":  `<![CDATA[<b>]]>Tellomi<![CDATA[</b>]]>`,
	}); err != nil {
		t.Fatal(err)
	}
	replaced, err := cat.Difference("android", "en", 4)
	if err != nil || replaced.GetFromVersion() != 0 || len(replaced.GetStrings()) != len(diff.GetStrings()) {
		t.Fatalf("below fix %+v %v", replaced, err)
	}
	caught, err := cat.Difference("android", "en", 5)
	if err != nil || len(caught.GetStrings()) != 0 || caught.GetFromVersion() != 5 {
		t.Fatalf("at fix %+v %v", caught, err)
	}
}

func parseAndroidLocaleXML(values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n")
	for _, key := range keys {
		buf.WriteString(`<string name="`)
		buf.WriteString(key)
		buf.WriteString(`">`)
		buf.WriteString(androidEscapeString(values[key]))
		buf.WriteString("</string>\n")
	}
	buf.WriteString("</resources>")
	dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.Strict = true
	for {
		if _, err := dec.Token(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
