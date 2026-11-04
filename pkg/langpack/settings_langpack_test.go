package langpack

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// settingsRow is one Tellomi settings string. English and simplified Chinese
// are the product copy from Android TellomiPack. Traditional Chinese is the
// s2twp wording (帳號、支援、手機號).
type settingsRow struct {
	key  string
	en   string
	hans string
	hant string
}

// tellomiSettingsRows is the M1 settings copy. Other languages are machine
// translations marked mt:true in the pack files.
var tellomiSettingsRows = []settingsRow{
	{"Tellomi.Settings.Group.Account", "Account", "账号", "帳號"},
	{"Tellomi.Settings.Group.General", "General", "通用", "通用"},
	{"Tellomi.Settings.Group.Support", "Support", "支持", "支援"},
	{"Tellomi.Settings.Help", "Help Center", "帮助中心", "幫助中心"},
	{"Tellomi.Settings.Card.Phone", "Phone", "手机号", "手機號"},
	{"Tellomi.Settings.Card.Email", "Email", "邮箱", "郵箱"},
	{"Tellomi.Settings.Card.Username", "Username", "用户名", "用戶名"},
}

func TestTellomiSettingsKeys(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	if len(tellomiSettingsRows) != 7 {
		t.Fatalf("settings keys %d", len(tellomiSettingsRows))
	}
	keys := make([]string, len(tellomiSettingsRows))
	for i, row := range tellomiSettingsRows {
		keys[i] = row.key
	}
	for _, platform := range []string{"ios", "android"} {
		for _, code := range []string{"en", "zh-hans", "zh-hant", "fr", "ja", "de"} {
			ver, ok := cat.Version(platform, code)
			if !ok || ver < 2 {
				t.Fatalf("%s %s version %d", platform, code, ver)
			}
			got, err := cat.Strings(platform, code, keys)
			if err != nil {
				t.Fatal(platform, code, err)
			}
			if len(got) != len(keys) {
				t.Fatalf("%s %s strings %d", platform, code, len(got))
			}
			for i, s := range got {
				row := tellomiSettingsRows[i]
				if s.GetPredicateName() != mtproto.Predicate_langPackString || s.GetKey() != row.key || s.GetValue() == "" {
					t.Fatalf("%s %s %s %+v", platform, code, row.key, s)
				}
				switch code {
				case "en":
					if s.GetValue() != row.en {
						t.Fatalf("%s %s got %q", platform, row.key, s.GetValue())
					}
				case "zh-hans":
					if s.GetValue() != row.hans {
						t.Fatalf("%s %s got %q", platform, row.key, s.GetValue())
					}
				case "zh-hant":
					if s.GetValue() != row.hant {
						t.Fatalf("%s %s got %q", platform, row.key, s.GetValue())
					}
				case "fr":
					if row.key == "Tellomi.Settings.Group.Account" && s.GetValue() != "Compte" {
						t.Fatalf("%s fr account %q", platform, s.GetValue())
					}
				case "ja":
					if row.key == "Tellomi.Settings.Group.Account" && s.GetValue() != "アカウント" {
						t.Fatalf("%s ja account %q", platform, s.GetValue())
					}
				}
			}
			diff, err := cat.Difference(platform, code, ver-1)
			if err != nil {
				t.Fatal(platform, code, err)
			}
			if diff.GetVersion() != ver {
				t.Fatalf("%s %s difference version %d want %d", platform, code, diff.GetVersion(), ver)
			}
			seen := map[string]struct{}{}
			for _, s := range diff.GetStrings() {
				seen[s.GetKey()] = struct{}{}
			}
			for _, key := range keys {
				if _, ok := seen[key]; !ok {
					t.Fatalf("%s %s getDifference missed %s", platform, code, key)
				}
			}
			caught, err := cat.Difference(platform, code, ver)
			if err != nil || len(caught.GetStrings()) != 0 {
				t.Fatalf("%s %s caught up %+v %v", platform, code, caught, err)
			}
		}
		raw := readPackMT(t, "../../data/langpack/"+platform+"/fr.json.gz")
		for _, row := range tellomiSettingsRows {
			item, ok := raw[row.key]
			if !ok || !item.mt || item.via == "" || item.value == "" {
				t.Fatalf("%s fr %s %+v", platform, row.key, item)
			}
		}
		en := readPackMT(t, "../../data/langpack/"+platform+"/en.json.gz")
		for _, row := range tellomiSettingsRows {
			item := en[row.key]
			if item.mt || item.value != row.en {
				t.Fatalf("%s en %s %+v", platform, row.key, item)
			}
		}
	}
}

type packMTItem struct {
	value string
	mt    bool
	via   string
}

func readPackMT(t *testing.T, path string) map[string]packMTItem {
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
		Strings []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			MT    bool   `json:"mt"`
			Via   string `json:"mt_via"`
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	out := make(map[string]packMTItem, len(doc.Strings))
	for _, s := range doc.Strings {
		out[s.Key] = packMTItem{value: s.Value, mt: s.MT, via: s.Via}
	}
	return out
}
