package langpack

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// tellomiClientManifest is data/langpack/tellomi-client-keys.json.
// base is master / release/ios. pull_requests lists keys named by an open
// client PR, including iOS PR 162's 105 Moments strings.
type tellomiClientManifest struct {
	Android tellomiPlatformKeys `json:"android"`
	IOS     tellomiPlatformKeys `json:"ios"`
}

type tellomiPlatformKeys struct {
	Base         []string            `json:"base"`
	PullRequests map[string][]string `json:"pull_requests"`
}

func (p tellomiPlatformKeys) all() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(keys []string) {
		for _, key := range keys {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	add(p.Base)
	for _, keys := range p.PullRequests {
		add(keys)
	}
	return out
}

func loadTellomiManifest(t *testing.T) tellomiClientManifest {
	t.Helper()
	body, err := os.ReadFile("../../data/langpack/tellomi-client-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc tellomiClientManifest
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestTellomiClientKeysInEveryPack fails when a Tellomi client key is missing
// from any Android or iOS language, or when a changed pack did not bump the
// string version to the pack version (so getDifference would skip it).
func TestTellomiClientKeysInEveryPack(t *testing.T) {
	doc := loadTellomiManifest(t)
	ios162 := doc.IOS.PullRequests["pr:162"]
	if len(ios162) != 105 {
		t.Fatalf("ios pr:162 keys %d, want 105", len(ios162))
	}
	androidKeys := doc.Android.all()
	iosKeys := doc.IOS.all()
	if len(androidKeys) < 100 || len(iosKeys) < 100 {
		t.Fatalf("android keys %d ios keys %d", len(androidKeys), len(iosKeys))
	}
	// Tab captions the menu screen already uses. MainTabs* and Settings are
	// official strings. Moments_Title is Tellomi-only.
	for _, key := range []string{"Moments_Title", "MainTabsChats", "MainTabsProfile", "MainTabsContacts", "MainTabsCalls"} {
		if !containsKey(androidKeys, key) && key == "Moments_Title" {
			t.Fatalf("android manifest missing %s", key)
		}
	}

	for _, tc := range []struct {
		platform string
		keys     []string
	}{
		{"android", androidKeys},
		{"ios", iosKeys},
	} {
		dir := filepath.Join("..", "..", "data", "langpack", tc.platform)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var packs int
		for _, ent := range entries {
			name := ent.Name()
			if ent.IsDir() || len(name) < len(".json.gz") || name[len(name)-len(".json.gz"):] != ".json.gz" {
				continue
			}
			packs++
			code := name[:len(name)-len(".json.gz")]
			pack := readPackVersions(t, filepath.Join(dir, name))
			if pack.lang != code || pack.plural != pluralCodeByLang[code] {
				t.Fatalf("%s %s lang %q plural %q", tc.platform, code, pack.lang, pack.plural)
			}
			if tc.platform == "android" {
				if pack.version != 61730402 || pack.fix != 61730400 {
					t.Fatalf("android %s version %d fix %d", code, pack.version, pack.fix)
				}
			}
			if tc.platform == "ios" {
				want, ok := iosPackVersion[code]
				if !ok || pack.version != want {
					t.Fatalf("ios %s version %d want %d", code, pack.version, want)
				}
			}
			for _, key := range tc.keys {
				ver, ok := pack.strings[key]
				if !ok {
					t.Fatalf("%s %s missing %s", tc.platform, code, key)
				}
				// Changed strings are at the pack version. Strings this fill
				// left alone stay one version behind, which is the previous pack.
				if ver != pack.version && ver != pack.version-1 {
					t.Fatalf("%s %s %s string version %d pack %d", tc.platform, code, key, ver, pack.version)
				}
			}
			for _, key := range []string{"MainTabsChats", "MainTabsProfile", "MainTabsContacts", "MainTabsCalls"} {
				if tc.platform != "android" {
					continue
				}
				if _, ok := pack.strings[key]; !ok {
					t.Fatalf("android %s missing %s", code, key)
				}
			}
		}
		if packs < 30 {
			t.Fatalf("%s packs %d", tc.platform, packs)
		}
	}

	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	checks := []struct {
		platform string
		code     string
		key      string
		value    string
	}{
		{"android", "en", "TellomiMenuSettings", "Menu settings"},
		{"android", "en", "MainTabsMenuSettings", "Menu settings"},
		{"android", "en", "SettingsTools", "Tools"},
		{"android", "en", "MainTabsHideLabels", "Hide button labels"},
		{"android", "en", "MainTabsButtons", "Navigation bar buttons"},
		{"android", "zh-hans", "TellomiMenuSettings", "选单设定"},
		{"android", "zh-hans", "MainTabsMenuSettings", "选单设定"},
		{"android", "zh-hans", "SettingsTools", "工具"},
		{"android", "zh-hans", "MainTabsHideLabels", "隐藏按钮文字"},
		{"android", "zh-hans", "MainTabsButtons", "导航栏按钮"},
		{"android", "zh-hans", "Moments_Title", "动态"},
		{"android", "zh-hans", "Moments_FollowingEntry", "关注"},
		{"android", "zh-hant", "TellomiMenuSettings", "選單設定"},
		{"android", "zh-hant", "MainTabsMenuSettings", "選單設定"},
		{"android", "zh-hant", "MainTabsHideLabels", "隱藏按鈕文字"},
		{"android", "de", "TellomiMenuSettings", "Menüeinstellungen"},
		{"ios", "en", "Tellomi.MenuSettings.Title", "Menu settings"},
		{"ios", "en", "Tellomi.MenuSettings.HideButtonTitles", "Hide button titles"},
		{"ios", "en", "Moments.Title", "Moments"},
		{"ios", "zh-hans", "Tellomi.MenuSettings.Title", "选单设定"},
		{"ios", "zh-hans", "Moments.Title", "动态"},
		{"ios", "zh-hans", "Moments.Followers", "粉丝"},
		{"ios", "zh-hant", "Tellomi.MenuSettings.Title", "選單設定"},
		{"ios", "zh-hant", "Moments.Title", "動態"},
		{"ios", "zh-hant", "Moments.Followers", "粉絲"},
	}
	for _, tc := range checks {
		got, err := cat.Strings(tc.platform, tc.code, []string{tc.key})
		if err != nil || len(got) != 1 || got[0].GetPredicateName() != mtproto.Predicate_langPackString || got[0].GetValue() != tc.value {
			value := ""
			if err == nil && len(got) == 1 {
				value = got[0].GetValue()
			}
			t.Fatalf("%s %s %s got %q (%v) want %q", tc.platform, tc.code, tc.key, value, err, tc.value)
		}
	}
	// zh-hans and zh-hant must not keep the English text for a Tellomi key
	// that has Chinese. A string that is the same in Chinese (PDF) may match.
	for _, platform := range []string{"android", "ios"} {
		keys := androidKeys
		if platform == "ios" {
			keys = iosKeys
		}
		enText := readPackText(t, "../../data/langpack/"+platform+"/en.json.gz")
		hansText := readPackText(t, "../../data/langpack/"+platform+"/zh-hans.json.gz")
		hantText := readPackText(t, "../../data/langpack/"+platform+"/zh-hant.json.gz")
		for _, key := range keys {
			en, hans, hant := enText[key], hansText[key], hantText[key]
			if hans == "" || hant == "" {
				t.Fatalf("%s %s empty hans %q hant %q", platform, key, hans, hant)
			}
			if hans == en && hant == en {
				continue
			}
			if hans == en || hant == en {
				t.Fatalf("%s %s still English hans %q hant %q en %q", platform, key, hans, hant, en)
			}
		}
	}
	if hans := readPackText(t, "../../data/langpack/ios/zh-hans.json.gz"); hans["Story.Privacy.CategoryMutualFollowers"] != "互关好友" || hans["Story.Privacy.WhoCanViewMoment"] != "谁可以看" {
		t.Fatalf("ios story hans needs review copy: %+v", []string{hans["Story.Privacy.CategoryMutualFollowers"], hans["Story.Privacy.WhoCanViewMoment"]})
	}
	if hant := readPackText(t, "../../data/langpack/ios/zh-hant.json.gz"); hant["Story.Privacy.CategoryMutualFollowers"] != "互關好友" || hant["Story.Privacy.WhoCanViewMoment"] != "誰可以看" {
		t.Fatalf("ios story hant needs review copy: %+v", []string{hant["Story.Privacy.CategoryMutualFollowers"], hant["Story.Privacy.WhoCanViewMoment"]})
	}
	assertTellomiNotEnglish(t, "android", androidKeys)
	assertTellomiNotEnglish(t, "ios", iosKeys)

	ver, ok := cat.Version("android", "zh-hans")
	if !ok || ver != 61730402 {
		t.Fatalf("android zh-hans version %d", ver)
	}
	// 61730400 is the full-replace cutover. A phone still there receives the
	// Tellomi strings as an incremental difference, not a new file.
	diff, err := cat.Difference("android", "zh-hans", 61730400)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetFromVersion() != 61730400 || diff.GetVersion() != ver || len(diff.GetStrings()) == 0 {
		t.Fatalf("android cutover diff from %d version %d n %d", diff.GetFromVersion(), diff.GetVersion(), len(diff.GetStrings()))
	}
	seen := map[string]string{}
	for _, s := range diff.GetStrings() {
		if s.GetPredicateName() == mtproto.Predicate_langPackString {
			seen[s.GetKey()] = s.GetValue()
		}
	}
	if seen["MainTabsMenuSettings"] != "选单设定" || seen["TellomiMenuSettings"] != "选单设定" || seen["Moments_Title"] != "动态" {
		t.Fatalf("android getDifference menu %+v", []string{seen["MainTabsMenuSettings"], seen["TellomiMenuSettings"], seen["Moments_Title"]})
	}
	// Chinese had no new wording at 61730402, so a phone already at 61730401
	// is caught up. Japanese was still English and must receive the translation.
	caughtZh, err := cat.Difference("android", "zh-hans", 61730401)
	if err != nil || len(caughtZh.GetStrings()) != 0 || caughtZh.GetFromVersion() != 61730401 {
		t.Fatalf("android zh caught up %+v %v", caughtZh, err)
	}
	ja, err := cat.Difference("android", "ja", 61730401)
	if err != nil {
		t.Fatal(err)
	}
	if ja.GetFromVersion() != 61730401 || ja.GetVersion() != ver || len(ja.GetStrings()) == 0 {
		t.Fatalf("android ja diff from %d n %d", ja.GetFromVersion(), len(ja.GetStrings()))
	}
	jaTitle := ""
	for _, s := range ja.GetStrings() {
		if s.GetKey() == "Moments_Title" {
			jaTitle = s.GetValue()
		}
	}
	if jaTitle != "モーメント" {
		t.Fatalf("android ja Moments_Title %q", jaTitle)
	}
	jaOld, err := cat.Difference("android", "ja", 61730400)
	if err != nil || jaOld.GetFromVersion() != 61730400 {
		t.Fatalf("android ja cutover from %d %v", jaOld.GetFromVersion(), err)
	}
	caught, err := cat.Difference("android", "zh-hans", ver)
	if err != nil || len(caught.GetStrings()) != 0 {
		t.Fatalf("android caught up %+v %v", caught, err)
	}

	iosVer, ok := cat.Version("ios", "zh-hans")
	if !ok || iosVer != 22454352 {
		t.Fatalf("ios zh-hans version %d", iosVer)
	}
	// The Chinese Moments title did not change, so it stays at the previous
	// string version. A client one behind that string still receives it.
	iosDiff, err := cat.Difference("ios", "zh-hans", 22454350)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range iosDiff.GetStrings() {
		if s.GetKey() == "Moments.Title" && s.GetValue() == "动态" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ios getDifference missed Moments.Title")
	}
	iosCaught, err := cat.Difference("ios", "zh-hans", iosVer)
	if err != nil || len(iosCaught.GetStrings()) != 0 {
		t.Fatalf("ios zh caught up %+v %v", iosCaught, err)
	}
	jaIOS, err := cat.Difference("ios", "ja", 22418204)
	if err != nil {
		t.Fatal(err)
	}
	jaMoments := ""
	for _, s := range jaIOS.GetStrings() {
		if s.GetKey() == "Moments.Title" {
			jaMoments = s.GetValue()
		}
	}
	if jaIOS.GetFromVersion() != 22418204 || jaMoments != "モーメント" {
		t.Fatalf("ios ja Moments.Title %q from %d", jaMoments, jaIOS.GetFromVersion())
	}
	hantVer, ok := cat.Version("ios", "zh-hant")
	if !ok || hantVer != 22421537 {
		t.Fatalf("ios zh-hant version %d", hantVer)
	}
}

// englishAllowlist is data/langpack/tellomi-english-allowlist.json.
// brands match the whole value. entries name one key a language keeps in English.
type englishAllowlist struct {
	Brands  []string `json:"brands"`
	Entries []struct {
		Platform string `json:"platform"`
		Code     string `json:"code"`
		Key      string `json:"key"`
		Value    string `json:"value"`
		Reason   string `json:"reason"`
	} `json:"entries"`
}

func assertTellomiNotEnglish(t *testing.T, platform string, keys []string) {
	t.Helper()
	body, err := os.ReadFile("../../data/langpack/tellomi-english-allowlist.json")
	if err != nil {
		t.Fatal(err)
	}
	var allow englishAllowlist
	if err := json.Unmarshal(body, &allow); err != nil {
		t.Fatal(err)
	}
	brands := map[string]struct{}{}
	for _, brand := range allow.Brands {
		if brand != "PDF" && brand != "Tellomi" && brand != "TELLOMI" {
			t.Fatalf("brand %q is not a documented name", brand)
		}
		brands[brand] = struct{}{}
	}
	entries := map[string]string{}
	for _, row := range allow.Entries {
		if row.Reason == "" {
			t.Fatalf("allowlist entry %s %s %s has no reason", row.Platform, row.Code, row.Key)
		}
		entries[row.Platform+"\x00"+row.Code+"\x00"+row.Key] = row.Value
	}
	dir := filepath.Join("..", "..", "data", "langpack", platform)
	en := readPackText(t, filepath.Join(dir, "en.json.gz"))
	entriesDir, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for _, ent := range entriesDir {
		name := ent.Name()
		if ent.IsDir() || len(name) < len(".json.gz") || name[len(name)-len(".json.gz"):] != ".json.gz" {
			continue
		}
		code := name[:len(name)-len(".json.gz")]
		if code == "en" {
			continue
		}
		text := readPackText(t, filepath.Join(dir, name))
		for _, key := range keys {
			value, english := text[key], en[key]
			if value != english {
				continue
			}
			id := platform + "\x00" + code + "\x00" + key
			if _, ok := brands[value]; ok {
				if want, listed := entries[id]; listed {
					if want != value {
						t.Fatalf("allowlist value drifted %s %s %s", platform, code, key)
					}
					seen[id] = struct{}{}
				}
				continue
			}
			if entries[id] != value {
				t.Fatalf("%s %s %s still English %q", platform, code, key, value)
			}
			seen[id] = struct{}{}
		}
	}
	for _, row := range allow.Entries {
		if row.Platform != platform {
			continue
		}
		id := row.Platform + "\x00" + row.Code + "\x00" + row.Key
		if _, ok := seen[id]; !ok {
			t.Fatalf("allowlist entry is not an English copy anymore: %s %s %s", row.Platform, row.Code, row.Key)
		}
	}
}

func containsKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

type packVersions struct {
	version int32
	fix     int32
	lang    string
	plural  string
	strings map[string]int32
}

// iosPackVersion is one past the versions shipped in #297.
var iosPackVersion = map[string]int32{
	"ar": 22452758, "be": 22420170, "ca": 22412798, "cs": 22418779,
	"de": 22431602, "el": 22202259, "en": 22347350, "es": 22412200,
	"fa": 22437413, "fi": 22410854, "fr": 22391559, "he": 22452942,
	"hi": 22202259, "id": 22321175, "it": 22388318, "ja": 22418205,
	"ko": 22454170, "ms": 22352691, "nb": 22420483, "nl": 22449822,
	"pl": 22392158, "pt-br": 22431433, "ro": 22444907, "ru": 22421467,
	"sv": 22427914, "th": 22202259, "tr": 22434109, "uk": 22460356,
	"uz": 22449133, "vi": 22347890, "zh-hans": 22454352, "zh-hant": 22421537,
}

// pluralCodeByLang is the plural_code each pack already ships. This fill must
// not change it. pt-br uses pt. Chinese uses zh.
var pluralCodeByLang = map[string]string{
	"en": "en", "ar": "ar", "be": "be", "ca": "ca", "cs": "cs", "de": "de",
	"el": "el", "es": "es", "fa": "fa", "fi": "fi", "fr": "fr", "he": "he",
	"hi": "hi", "id": "id", "it": "it", "ja": "ja", "ko": "ko", "ms": "ms",
	"nb": "nb", "nl": "nl", "pl": "pl", "pt-br": "pt", "ro": "ro", "ru": "ru",
	"sv": "sv", "th": "th", "tr": "tr", "uk": "uk", "uz": "uz", "vi": "vi",
	"zh-hans": "zh", "zh-hant": "zh",
}

func readPackVersions(t *testing.T, path string) packVersions {
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
		Lang    string `json:"lang_code"`
		Plural  string `json:"plural_code"`
		Version int32  `json:"version"`
		Fix     int32  `json:"android_xml_fix_version"`
		Strings []struct {
			Key     string `json:"key"`
			Version int32  `json:"version"`
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(path, err)
	}
	out := packVersions{version: doc.Version, fix: doc.Fix, lang: doc.Lang, plural: doc.Plural, strings: make(map[string]int32, len(doc.Strings))}
	for _, s := range doc.Strings {
		out.strings[s.Key] = s.Version
	}
	return out
}

func readPackText(t *testing.T, path string) map[string]string {
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
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(path, err)
	}
	out := make(map[string]string, len(doc.Strings))
	for _, s := range doc.Strings {
		out[s.Key] = s.Value
	}
	return out
}
