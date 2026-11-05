package langpack

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func fixtureCatalog() *Catalog {
	one := "1 message"
	other := "%d messages"
	zero := "no messages"
	return NewCatalog(map[string]map[string]*Language{
		"android": {
			"en": {
				Code: "en", Name: "English", NativeName: "English", PluralCode: "en", Official: true,
				TranslationsURL: "https://tellomi.app/translations/android/en", Version: 2,
				Strings: []Entry{
					{Key: "AppName", Value: "Tellomi", Version: 1},
					{Key: "Settings", Value: "Settings", Version: 2},
					{Key: "Msg", Version: 2, Plural: &Plural{Zero: zero, One: one, Other: other}},
				},
			},
			"zh-hans": {
				Code: "zh-hans", Name: "Chinese (Simplified)", NativeName: "简体中文", PluralCode: "zh",
				Official: true, BaseLangCode: "en",
				TranslationsURL: "https://tellomi.app/translations/android/zh-hans", Version: 2,
				Strings: []Entry{
					{Key: "AppName", Value: "Tellomi", Version: 1},
					{Key: "Settings", Value: "设置", Version: 2},
					{Key: "Msg", Version: 1, Plural: &Plural{Other: "%d 条消息"}},
				},
			},
			"zh-hant": {
				Code: "zh-hant", Name: "Chinese (Traditional)", NativeName: "繁體中文", PluralCode: "zh",
				Official: true, BaseLangCode: "en", Version: 1,
				Strings: []Entry{{Key: "AppName", Value: "Tellomi", Version: 1}},
			},
		},
		"ios": {
			"en": {
				Code: "en", Name: "English", NativeName: "English", PluralCode: "en", Official: true, Version: 1,
				Strings: []Entry{
					{Key: "LastSeen.MinutesAgo", Version: 1, Plural: &Plural{Zero: "last seen %@ minutes ago", One: "last seen 1 minute ago", Other: "last seen %@ minutes ago"}},
				},
			},
			"zh-hans": {
				Code: "zh-hans", Name: "Chinese (Simplified)", NativeName: "简体中文", PluralCode: "zh",
				Official: true, BaseLangCode: "en", Version: 1,
				Strings: []Entry{
					{Key: "LastSeen.MinutesAgo", Version: 1, Plural: &Plural{Other: "最后上线于 %@ 分钟前"}},
				},
			},
			"zh-hant": {
				Code: "zh-hant", Name: "Chinese (Traditional)", NativeName: "繁體中文", PluralCode: "zh",
				Official: true, BaseLangCode: "en", Version: 1,
				Strings: []Entry{{Key: "Settings.Title", Value: "設定", Version: 1}},
			},
		},
	})
}

func TestLanguagesMetadata(t *testing.T) {
	cat := fixtureCatalog()
	langs, err := cat.Languages("android")
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 3 {
		t.Fatalf("languages = %d", len(langs))
	}
	if langs[0].GetLangCode() != "zh-hans" || langs[0].GetNativeName() != "简体中文" {
		t.Fatalf("first = %+v", langs[0])
	}
	if langs[0].GetPluralCode() != "zh" || !langs[0].GetOfficial() {
		t.Fatalf("plural/official = %s %v", langs[0].GetPluralCode(), langs[0].GetOfficial())
	}
	if langs[0].GetBaseLangCode().GetValue() != "en" {
		t.Fatalf("base = %v", langs[0].GetBaseLangCode())
	}
	if langs[0].GetStringsCount() < langs[0].GetTranslatedCount() || langs[0].GetTranslatedCount() != 3 {
		t.Fatalf("counts %d/%d", langs[0].GetTranslatedCount(), langs[0].GetStringsCount())
	}
	if !strings.Contains(langs[0].GetTranslationsUrl(), "zh-hans") {
		t.Fatal(langs[0].GetTranslationsUrl())
	}
	if langs[2].GetLangCode() != "en" || langs[2].GetPluralCode() != "en" {
		t.Fatalf("en = %+v", langs[2])
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := langs[0].Encode(buf, 232); err != nil {
		t.Fatal(err)
	}

	if _, err := cat.Languages("unknown"); err != ErrUnknownPack {
		t.Fatal(err)
	}
	if _, err := cat.Language("android", "fr"); err != ErrUnknownLang {
		t.Fatal(err)
	}
	one, err := cat.Language("ios", "zh_HK")
	if err != nil || one.GetLangCode() != "zh-hant" || one.GetNativeName() != "繁體中文" {
		t.Fatalf("alias language: %+v %v", one, err)
	}
}

func TestDifferenceAndStrings(t *testing.T) {
	cat := fixtureCatalog()
	full, err := cat.Difference("android", "zh", 0)
	if err != nil {
		t.Fatal(err)
	}
	if full.GetLangCode() != "zh-hans" || full.GetFromVersion() != 0 || full.GetVersion() != 2 {
		t.Fatalf("full header %+v", full)
	}
	if len(full.GetStrings()) != 3 {
		t.Fatalf("full strings %d", len(full.GetStrings()))
	}
	var sawPlural bool
	for _, s := range full.GetStrings() {
		if s.GetKey() == "Msg" {
			if s.GetPredicateName() != mtproto.Predicate_langPackStringPluralized || s.GetOtherValue() != "%d 条消息" || s.GetOneValue() != nil {
				t.Fatalf("plural = %+v", s)
			}
			sawPlural = true
			buf := mtproto.NewEncodeBuf(128)
			if err := s.Encode(buf, 232); err != nil {
				t.Fatal(err)
			}
		}
		if s.GetKey() == "AppName" && s.GetValue() != "Tellomi" {
			t.Fatal(s.GetValue())
		}
	}
	if !sawPlural {
		t.Fatal("missing plural")
	}

	diff, err := cat.Difference("android", "en", 1)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetVersion() != 2 || len(diff.GetStrings()) != 2 {
		t.Fatalf("diff version %d strings %d", diff.GetVersion(), len(diff.GetStrings()))
	}
	for _, s := range diff.GetStrings() {
		if s.GetKey() == "AppName" {
			t.Fatal("AppName should stay at version 1")
		}
	}
	same, err := cat.Difference("android", "en", 2)
	if err != nil || len(same.GetStrings()) != 0 || same.GetVersion() != 2 {
		t.Fatalf("empty diff %+v %v", same, err)
	}

	got, err := cat.Strings("android", "zh-hans", []string{"Settings", "Msg_other", "Missing"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].GetValue() != "设置" {
		t.Fatal(got[0])
	}
	if got[1].GetValue() != "%d 条消息" || got[1].GetPredicateName() != mtproto.Predicate_langPackString {
		t.Fatalf("suffix = %+v", got[1])
	}
	if got[2].GetPredicateName() != mtproto.Predicate_langPackStringDeleted || got[2].GetKey() != "Missing" {
		t.Fatalf("deleted = %+v", got[2])
	}

	ios, err := cat.Difference("ios", "en", 0)
	if err != nil {
		t.Fatal(err)
	}
	var zero bool
	for _, s := range ios.GetStrings() {
		if s.GetKey() == "LastSeen.MinutesAgo_0" && s.GetValue() == "last seen %@ minutes ago" {
			zero = true
		}
		if s.GetKey() == "LastSeen.MinutesAgo" && s.GetPredicateName() != mtproto.Predicate_langPackStringPluralized {
			t.Fatal("ios plural missing")
		}
	}
	if !zero {
		t.Fatal("ios zero suffix missing")
	}
}

func TestApplyConfigLanguage(t *testing.T) {
	inner := mtproto.MakeTLConfig(&mtproto.Config{
		SuggestedLangCode: &wrapperspb.StringValue{Value: "classic-zh-cn"},
	})
	ApplyConfigLanguage(inner, "en", "zh-CN", "ios")
	if inner.GetSuggestedLangCode().GetValue() != "zh-hans" {
		t.Fatalf("suggested = %s", inner.GetSuggestedLangCode().GetValue())
	}
	if inner.GetLangPackVersion().GetValue() < 1 {
		t.Fatal("missing lang pack version")
	}
	ApplyConfigLanguage(inner, "en", "en-US", "android")
	if inner.GetSuggestedLangCode().GetValue() != "en" {
		t.Fatalf("en suggested = %s", inner.GetSuggestedLangCode().GetValue())
	}
	enVer, ok := DefaultCatalog().Version("android", "en")
	ApplyConfigLanguage(inner, "en", "fr-FR", "android")
	if !ok || inner.GetSuggestedLangCode().GetValue() != "fr" || inner.GetLangPackVersion().GetValue() != enVer {
		t.Fatalf("suggested %s version %d want fr %d", inner.GetSuggestedLangCode().GetValue(), inner.GetLangPackVersion().GetValue(), enVer)
	}
	zhVer, ok := DefaultCatalog().Version("android", "zh-hans")
	ApplyConfigLanguage(inner, "zh-hans", "de", "android")
	if !ok || inner.GetSuggestedLangCode().GetValue() != "de" || inner.GetLangPackVersion().GetValue() != zhVer {
		t.Fatalf("app pack version suggested %s version %d want de %d", inner.GetSuggestedLangCode().GetValue(), inner.GetLangPackVersion().GetValue(), zhVer)
	}
}

func offeredOfficialCount() int {
	n := 0
	for _, lang := range OfficialLanguages {
		if !HiddenLang(lang.Code) {
			n++
		}
	}
	return n
}

func TestShippedPacks(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	for _, platform := range []string{"ios", "android"} {
		langs, err := cat.Languages(platform)
		if err != nil {
			t.Fatal(platform, err)
		}
		if len(langs) != offeredOfficialCount() {
			t.Fatalf("%s languages %d", platform, len(langs))
		}
		byCode := map[string]*mtproto.LangPackLanguage{}
		for _, lang := range langs {
			byCode[lang.GetLangCode()] = lang
			if lang.GetNativeName() == "" || lang.GetPluralCode() == "" || lang.GetTranslationsUrl() == "" || !lang.GetOfficial() {
				t.Fatalf("%s incomplete %+v", platform, lang)
			}
			if lang.GetStringsCount() < 1000 {
				t.Fatalf("%s %s counts %d/%d", platform, lang.GetLangCode(), lang.GetTranslatedCount(), lang.GetStringsCount())
			}
			if lang.GetTranslatedCount() < minTranslated(platform, lang.GetLangCode()) {
				t.Fatalf("%s %s translated %d", platform, lang.GetLangCode(), lang.GetTranslatedCount())
			}
			if lang.GetLangCode() == "en" {
				if lang.GetBaseLangCode().GetValue() != "" {
					t.Fatalf("%s en base %q", platform, lang.GetBaseLangCode().GetValue())
				}
			} else if lang.GetBaseLangCode().GetValue() != "en" {
				t.Fatalf("%s %s base %q", platform, lang.GetLangCode(), lang.GetBaseLangCode().GetValue())
			}
			diff, err := cat.Difference(platform, lang.GetLangCode(), 0)
			if err != nil || diff.GetVersion() < 1 {
				t.Fatalf("%s %s diff %v %v", platform, lang.GetLangCode(), diff.GetVersion(), err)
			}
			buf := mtproto.NewEncodeBuf(256)
			if err := lang.Encode(buf, 232); err != nil {
				t.Fatal(err)
			}
			if len(diff.GetStrings()) > 0 {
				if err := diff.GetStrings()[0].Encode(buf, 232); err != nil {
					t.Fatal(err)
				}
			}
			for _, s := range diff.GetStrings() {
				if hasTelegramBrand(s.GetValue()) || hasTelegramBrand(s.GetOtherValue()) {
					t.Fatalf("brand in %s %s %q", platform, s.GetKey(), s.GetValue()+s.GetOtherValue())
				}
				for _, part := range stringParts(s) {
					low := strings.ToLower(part)
					if strings.Contains(low, "t.me") || strings.Contains(low, "telegram.org") || strings.Contains(low, "telegram.me") || strings.Contains(low, "telegram.dog") {
						t.Fatalf("telegram host in %s %s %q", platform, s.GetKey(), part)
					}
				}
			}
		}
		if byCode["zh-hans"].GetNativeName() != "简体中文" || byCode["zh-hans"].GetPluralCode() != "zh" {
			t.Fatal(byCode["zh-hans"])
		}
		if byCode["zh-hant"].GetNativeName() != "繁體中文" || byCode["zh-hant"].GetPluralCode() != "zh" {
			t.Fatal(byCode["zh-hant"])
		}
		if byCode["en"].GetPluralCode() != "en" {
			t.Fatal(byCode["en"])
		}
		if byCode["zh-hans"].GetBaseLangCode().GetValue() != "en" {
			t.Fatal("missing base")
		}
	}

	android, err := cat.Strings("android", "zh-hans", []string{"AppName", "Settings", "ChooseYourLanguage"})
	if err != nil {
		t.Fatal(err)
	}
	if android[0].GetValue() != "Tellomi" || android[1].GetValue() != "设置" || android[2].GetValue() != "选择您的语言" {
		t.Fatalf("android alert strings %+v", android)
	}
	ios, err := cat.Strings("ios", "zh-hant", []string{"Settings.Title"})
	if err != nil || ios[0].GetValue() != "設定" {
		t.Fatalf("ios traditional %+v %v", ios, err)
	}

	// Official packs use the translations.telegram.org version, which is above
	// the old Tellomi-only versions (en=2, zh=3). A client cached at version 3
	// still receives the newly imported strings.
	for _, platform := range []string{"ios", "android", "tdesktop", "macos"} {
		langs, err := cat.Languages(platform)
		if err != nil {
			t.Fatal(platform, err)
		}
		if len(langs) != offeredOfficialCount() {
			t.Fatalf("%s languages %d", platform, len(langs))
		}
		for _, lang := range langs {
			if HiddenLang(lang.GetLangCode()) {
				t.Fatalf("%s lists hidden %s", platform, lang.GetLangCode())
			}
			if lang.GetTranslatedCount() < minTranslated(platform, lang.GetLangCode()) {
				t.Fatalf("%s %s translated %d", platform, lang.GetLangCode(), lang.GetTranslatedCount())
			}
			if lang.GetLangCode() == "en" {
				if lang.GetBaseLangCode().GetValue() != "" {
					t.Fatalf("%s en base %q", platform, lang.GetBaseLangCode().GetValue())
				}
			} else if lang.GetBaseLangCode().GetValue() != "en" {
				t.Fatalf("%s %s base %q", platform, lang.GetLangCode(), lang.GetBaseLangCode().GetValue())
			}
			diff, err := cat.Difference(platform, lang.GetLangCode(), 0)
			if err != nil || diff.GetVersion() < 1 {
				t.Fatalf("%s %s diff %v %v", platform, lang.GetLangCode(), diff.GetVersion(), err)
			}
			if int32(len(diff.GetStrings())) < lang.GetTranslatedCount() {
				t.Fatalf("%s %s pack %d < translated %d", platform, lang.GetLangCode(), len(diff.GetStrings()), lang.GetTranslatedCount())
			}
			caught, err := cat.Difference(platform, lang.GetLangCode(), diff.GetVersion())
			if err != nil || len(caught.GetStrings()) != 0 || caught.GetVersion() != diff.GetVersion() {
				t.Fatalf("%s %s caught up %+v %v", platform, lang.GetLangCode(), caught, err)
			}
			for _, s := range diff.GetStrings() {
				if strings.Contains(strings.ToLower(s.GetKey()), "tell.cc") {
					t.Fatalf("rewritten key name %s %s %q", platform, lang.GetLangCode(), s.GetKey())
				}
				if hasTelegramBrand(s.GetValue()) || hasTelegramBrand(s.GetOtherValue()) {
					t.Fatalf("brand in %s %s %q", platform, s.GetKey(), s.GetValue()+s.GetOtherValue())
				}
				for _, part := range stringParts(s) {
					low := strings.ToLower(part)
					if strings.Contains(low, "t.me") || strings.Contains(low, "telegram.org") || strings.Contains(low, "telegram.me") || strings.Contains(low, "telegram.dog") {
						t.Fatalf("telegram host in %s %s %q", platform, s.GetKey(), part)
					}
					code := lang.GetLangCode()
					if (platform == "ios" || platform == "android") && (code == "zh-hans" || code == "zh-hant") && (strings.Contains(s.GetKey(), "ell.cc") || strings.Contains(part, "](网页)") || strings.Contains(part, "](網頁)") || strings.Contains(part, "](Tellomi)") || strings.Contains(part, "何时你") || strings.Contains(part, "因此该你")) {
						t.Fatalf("bad overlay leftover %s %s %s %q", platform, code, s.GetKey(), part)
					}
				}
			}
		}
		switch platform {
		case "android":
			ja, err := cat.Strings(platform, "ja", []string{"AppName"})
			if err != nil || ja[0].GetValue() != "Tellomi" {
				t.Fatalf("%s ja AppName %+v %v", platform, ja, err)
			}
		case "ios":
			ja, err := cat.Strings(platform, "ja", []string{"Application.Name"})
			if err != nil || ja[0].GetValue() != "Tellomi" {
				t.Fatalf("%s ja Application.Name %+v %v", platform, ja, err)
			}
		case "tdesktop":
			ja, err := cat.Strings(platform, "ja", []string{"lng_language_name", "lng_menu_contacts"})
			if err != nil || ja[0].GetValue() != "日本語" || ja[1].GetValue() != "連絡先" {
				t.Fatalf("%s ja %+v %v", platform, ja, err)
			}
		case "macos":
			ja, err := cat.Strings(platform, "ja", []string{"Login.ContinueOnLanguage"})
			if err != nil || ja[0].GetValue() != "日本語で続ける" {
				t.Fatalf("%s ja %+v %v", platform, ja, err)
			}
		}
	}
	updated, err := cat.Difference("android", "zh-hans", 3)
	if err != nil || len(updated.GetStrings()) < 1000 || updated.GetVersion() <= 3 {
		t.Fatalf("zh-hans refresh from 3: %d v=%d %v", len(updated.GetStrings()), updated.GetVersion(), err)
	}
	jaName, err := cat.Strings("android", "ja", []string{"LanguageName", "Settings"})
	if err != nil || jaName[0].GetValue() != "日本語" || jaName[1].GetValue() != "設定" {
		t.Fatalf("ja %+v %v", jaName, err)
	}
	deName, err := cat.Strings("android", "de", []string{"LanguageName"})
	if err != nil || deName[0].GetValue() != "Deutsch" {
		t.Fatalf("de %+v", deName)
	}
	hiName, err := cat.Strings("android", "hi", []string{"Settings"})
	if err != nil || hiName[0].GetValue() != "सेटिंग्स" {
		t.Fatalf("hi %+v", hiName)
	}
	iosJA, err := cat.Strings("ios", "ja", []string{"Settings.Title", "Login.ContinueWithLocalization"})
	if err != nil || iosJA[0].GetValue() != "設定" || iosJA[1].GetValue() != "日本語で続ける" {
		t.Fatalf("ios ja %+v", iosJA)
	}
	iosHans, err := cat.Strings("ios", "zh-hans", []string{
		"InfoPlist.NSLocationAlwaysUsageDescription",
		"InfoPlist.NSLocationWhenInUseUsageDescription",
		"AuthSessions.HeaderInfo",
		"Login.ContinueWithLocalization",
		"AccessDenied.LocationDenied",
		"Chat.Message.TopicAuthorBadge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(iosHans[0].GetValue(), "何时") || strings.Contains(iosHans[1].GetValue(), "何时") {
		t.Fatalf("location permission copy is still garbled: %#v", iosHans[:2])
	}
	header := iosHans[2].GetValue()
	if !strings.Contains(header, "](web)") || !strings.Contains(header, "](desktop)") || strings.Contains(header, "](网页)") || strings.Contains(header, "](Tellomi)") {
		t.Fatalf("login link targets: %q", header)
	}
	if iosHans[3].GetValue() != "使用简体中文继续" {
		t.Fatalf("continue-in-language: %q", iosHans[3].GetValue())
	}
	if strings.Contains(iosHans[4].GetValue(), "因此该") {
		t.Fatalf("location denial still machine-translated: %q", iosHans[4].GetValue())
	}
	if iosHans[5].GetValue() == "" {
		t.Fatal("real Chat.Message key was lost")
	}
	iosEn, err := cat.Strings("ios", "en", []string{"Chat.Message.TopicAuthorBadge", "Chatell.ccssage.TopicAuthorBadge"})
	if err != nil || iosEn[0].GetValue() != "Topic Creator" {
		t.Fatalf("ios en real key %+v %v", iosEn, err)
	}
	if iosEn[1].GetPredicateName() != mtproto.Predicate_langPackStringDeleted {
		t.Fatalf("rewritten key still served %+v", iosEn[1])
	}
	refreshed, err := cat.Difference("ios", "zh-hans", 22454343)
	if err != nil || refreshed.GetVersion() < 22454346 || len(refreshed.GetStrings()) < 1000 {
		t.Fatalf("ios zh-hans bump %+v %v", refreshed.GetVersion(), err)
	}
	androidRefreshed, err := cat.Difference("android", "zh-hans", 61570949)
	if err != nil || androidRefreshed.GetVersion() < 61570953 || len(androidRefreshed.GetStrings()) < 1000 {
		t.Fatalf("android zh-hans bump %+v %v", androidRefreshed.GetVersion(), err)
	}
	hant, err := cat.Difference("ios", "zh-hant", 22421528)
	if err != nil || hant.GetVersion() < 22421531 || len(hant.GetStrings()) < 1000 {
		t.Fatalf("ios zh-hant bump v=%d n=%d %v", hant.GetVersion(), len(hant.GetStrings()), err)
	}
	hantAndroid, err := cat.Difference("android", "zh-hant", 61519200)
	if err != nil || hantAndroid.GetVersion() < 61519203 || len(hantAndroid.GetStrings()) < 1000 {
		t.Fatalf("android zh-hant bump v=%d n=%d %v", hantAndroid.GetVersion(), len(hantAndroid.GetStrings()), err)
	}

	classic, err := cat.Language("android", "classic-zh-cn")
	hans, hansErr := cat.Language("android", "zh-hans")
	if err != nil || hansErr != nil || classic.GetLangCode() != "zh-hans" || classic.GetBaseLangCode().GetValue() != "en" || classic.GetTranslatedCount() != hans.GetTranslatedCount() {
		t.Fatalf("classic fallback %+v hans %+v %v", classic, hans, err)
	}
	tw, err := cat.Language("ios", "taiwan")
	hantLang, hantErr := cat.Language("ios", "zh-hant")
	if err != nil || hantErr != nil || tw.GetLangCode() != "zh-hant" || tw.GetBaseLangCode().GetValue() != "en" || tw.GetNativeName() != "繁體中文" || tw.GetTranslatedCount() != hantLang.GetTranslatedCount() {
		t.Fatalf("taiwan fallback %+v hant %+v %v", tw, hantLang, err)
	}
	for _, platform := range []string{"ios", "android", "tdesktop", "macos"} {
		listed, err := cat.Languages(platform)
		if err != nil {
			t.Fatal(platform, err)
		}
		var simplified, traditional int
		for _, lang := range listed {
			switch lang.GetLangCode() {
			case "classic-zh-cn", "taiwan":
				t.Fatalf("%s still lists %s", platform, lang.GetLangCode())
			case "zh-hans":
				simplified++
			case "zh-hant":
				traditional++
			}
			if lang.GetNativeName() == "简体中文" && lang.GetLangCode() != "zh-hans" {
				t.Fatalf("%s extra simplified %+v", platform, lang)
			}
			if lang.GetNativeName() == "正體中文" {
				t.Fatalf("%s still lists 正體中文", platform)
			}
		}
		if simplified != 1 || traditional != 1 {
			t.Fatalf("%s chinese entries simplified=%d traditional=%d", platform, simplified, traditional)
		}
	}
	legacyStrings, err := cat.Strings("ios", "classic-zh-cn", []string{"Login.ContinueWithLocalization"})
	officialStrings, err2 := cat.Strings("ios", "zh-hans", []string{"Login.ContinueWithLocalization"})
	if err != nil || err2 != nil || legacyStrings[0].GetValue() == "" || legacyStrings[0].GetValue() != officialStrings[0].GetValue() {
		t.Fatalf("classic strings %+v %+v %v %v", legacyStrings, officialStrings, err, err2)
	}
	twStrings, err := cat.Strings("android", "taiwan", []string{"LanguageName"})
	hantStrings, err2 := cat.Strings("android", "zh-hant", []string{"LanguageName"})
	if err != nil || err2 != nil || twStrings[0].GetValue() == "" || twStrings[0].GetValue() != hantStrings[0].GetValue() {
		t.Fatalf("taiwan strings %+v %+v %v %v", twStrings, hantStrings, err, err2)
	}
}

// minTranslated is how many real strings a shipped pack must contain.
// Thai and Greek exports on several platforms are empty or a few English
// leftovers; Hindi on iOS is a partial pack. They still fall back to English.
func minTranslated(platform, code string) int32 {
	switch code {
	case "th", "el":
		return 0
	case "hi":
		if platform == "android" {
			return 1000
		}
		return 300
	default:
		return 1000
	}
}

func stringParts(s *mtproto.LangPackString) []string {
	parts := []string{s.GetValue(), s.GetOtherValue()}
	for _, opt := range []*wrapperspb.StringValue{s.GetZeroValue(), s.GetOneValue(), s.GetTwoValue(), s.GetFewValue(), s.GetManyValue()} {
		if opt != nil {
			parts = append(parts, opt.Value)
		}
	}
	return parts
}

func hasTelegramBrand(s string) bool {
	low := strings.ToLower(s)
	for i := 0; i+8 <= len(low); {
		j := strings.Index(low[i:], "telegram")
		if j < 0 {
			return false
		}
		j += i
		end := j + len("telegram")
		prevOK := j == 0 || !isWordByte(low[j-1])
		nextWord := end < len(low) && isWordByte(low[end])
		nextOrg := strings.HasPrefix(low[end:], ".org")
		if prevOK && !nextWord && !nextOrg {
			return true
		}
		i = end
	}
	return false
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func TestEmbeddedPacksLoad(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	for _, platform := range []string{"ios", "android"} {
		langs, err := cat.Languages(platform)
		if err != nil {
			t.Fatal(platform, err)
		}
		if len(langs) != offeredOfficialCount() {
			t.Fatalf("%s languages %d", platform, len(langs))
		}
		for _, lang := range langs {
			if HiddenLang(lang.GetLangCode()) {
				t.Fatalf("%s lists hidden %s", platform, lang.GetLangCode())
			}
			if lang.GetNativeName() == "" || lang.GetPluralCode() == "" || lang.GetTranslationsUrl() == "" {
				t.Fatalf("%s incomplete %+v", platform, lang)
			}
			diff, err := cat.Difference(platform, lang.GetLangCode(), 0)
			if err != nil || diff.GetVersion() < 1 {
				t.Fatalf("%s %s diff %v %v", platform, lang.GetLangCode(), diff, err)
			}
		}
	}
	// Empty lang_pack is resolved by the handler; the catalog itself requires a platform.
	if _, err := cat.Languages(""); err != ErrUnknownPack {
		t.Fatal(err)
	}
}

func TestChineseOverlayIsCurated(t *testing.T) {
	for _, item := range []struct {
		path string
		max  int
	}{
		// 12 curated lines, the Discover and sign-up keys, and 7 settings keys.
		{"../../data/langpack/overrides/ios/zh-hans.json.gz", 122},
		{"../../data/langpack/overrides/ios/zh-hant.json.gz", 122},
		// 3 curated lines, the shared Discover keys, the Android report keys, and 7 settings keys.
		{"../../data/langpack/overrides/android/zh-hans.json.gz", 123},
		{"../../data/langpack/overrides/android/zh-hant.json.gz", 123},
	} {
		fh, err := os.Open(item.path)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := gzip.NewReader(fh)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Strings []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"strings"`
		}
		if err := json.NewDecoder(zr).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		_ = zr.Close()
		_ = fh.Close()
		if len(doc.Strings) == 0 || len(doc.Strings) > item.max {
			t.Fatalf("%s overlay size %d", item.path, len(doc.Strings))
		}
		for _, s := range doc.Strings {
			if strings.Contains(s.Value, "](网页)") || strings.Contains(s.Value, "](網頁)") || strings.Contains(s.Value, "何时你") || strings.Contains(s.Value, "因此该") {
				t.Fatalf("%s %s %q", item.path, s.Key, s.Value)
			}
		}
	}
}

func TestLegacyCodeHiddenAndFallsBack(t *testing.T) {
	cat := NewCatalog(map[string]map[string]*Language{
		"android": {
			"zh-hans": {
				Code: "zh-hans", Name: "Chinese (Simplified)", NativeName: "简体中文", PluralCode: "zh",
				Official: true, BaseLangCode: "en", Version: 4,
				Strings: []Entry{{Key: "Settings", Value: "设置", Version: 4}},
			},
			"classic-zh-cn": {
				Code: "classic-zh-cn", Name: "Chinese (Simplified, Classic)", NativeName: "简体中文", PluralCode: "zh",
				Official: true, BaseLangCode: "zh-hans", Version: 1,
				Strings: []Entry{{Key: "Settings", Value: "旧简体", Version: 1}},
			},
			"zh-hant": {
				Code: "zh-hant", Name: "Chinese (Traditional)", NativeName: "繁體中文", PluralCode: "zh",
				Official: true, BaseLangCode: "en", Version: 4,
				Strings: []Entry{{Key: "Settings", Value: "設定", Version: 4}},
			},
			"taiwan": {
				Code: "taiwan", Name: "Chinese (Traditional, Taiwan)", NativeName: "正體中文", PluralCode: "zh",
				Official: true, BaseLangCode: "zh-hant", Version: 1,
				Strings: []Entry{{Key: "Settings", Value: "舊繁體", Version: 1}},
			},
		},
	})
	langs, err := cat.Languages("android")
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 2 || langs[0].GetLangCode() != "zh-hans" || langs[1].GetLangCode() != "zh-hant" {
		t.Fatalf("listed %+v", langs)
	}
	got, err := cat.Strings("android", "classic-zh-cn", []string{"Settings"})
	if err != nil || got[0].GetValue() != "设置" {
		t.Fatalf("classic strings %+v %v", got, err)
	}
	tw, err := cat.Strings("android", "taiwan", []string{"Settings"})
	if err != nil || tw[0].GetValue() != "設定" {
		t.Fatalf("taiwan strings %+v %v", tw, err)
	}
	diff, err := cat.Difference("android", "classic-zh-cn", 0)
	if err != nil || diff.GetLangCode() != "zh-hans" || diff.GetStrings()[0].GetValue() != "设置" {
		t.Fatalf("classic pack %+v %v", diff, err)
	}
}

func TestLoadSkipsLegacyPackFiles(t *testing.T) {
	dir := t.TempDir()
	writePack := func(name, code, value string) {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"lang_code":        code,
			"name":             code,
			"native_name":      code,
			"plural_code":      "zh",
			"official":         true,
			"base_lang_code":   "en",
			"translations_url": "https://tellomi.cn/translations/android/" + code,
			"version":          5,
			"strings":          []map[string]any{{"key": "Settings", "value": value, "version": 5}},
		})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "android", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		fh, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		zw := gzip.NewWriter(fh)
		if _, err := zw.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := fh.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Only the removed pack is on disk. It must not be served as zh-hans.
	writePack("classic-zh-cn.json.gz", "classic-zh-cn", "旧简体")
	cat, err := Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Languages("android"); err != ErrUnknownPack {
		t.Fatalf("legacy-only directory listed a language: %v", err)
	}
	if _, err := cat.Strings("android", "classic-zh-cn", []string{"Settings"}); err != ErrUnknownPack {
		t.Fatalf("legacy-only strings: %v", err)
	}

	writePack("zh-hans.json.gz", "zh-hans", "设置")
	writePack("taiwan.json.gz", "taiwan", "舊繁體")
	writePack("zh-hant.json.gz", "zh-hant", "設定")
	cat, err = Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	langs, err := cat.Languages("android")
	if err != nil || len(langs) != 2 {
		t.Fatalf("langs %d %v", len(langs), err)
	}
	got, err := cat.Strings("android", "classic-zh-cn", []string{"Settings"})
	if err != nil || got[0].GetValue() != "设置" {
		t.Fatalf("loaded classic %+v %v", got, err)
	}
	tw, err := cat.Strings("android", "taiwan", []string{"Settings"})
	if err != nil || tw[0].GetValue() != "設定" {
		t.Fatalf("loaded taiwan %+v %v", tw, err)
	}
}

func TestAndroidZhHansTellomiStrings(t *testing.T) {
	cat := DefaultCatalog()
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	diff, err := cat.Difference("android", "zh-hans", 61570950)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetVersion() < 61570953 {
		t.Fatalf("version %d", diff.GetVersion())
	}
	if len(diff.GetStrings()) < 100 {
		t.Fatalf("difference from 61570950 has %d strings", len(diff.GetStrings()))
	}
	seen := map[string]*mtproto.LangPackString{}
	for _, s := range diff.GetStrings() {
		seen[s.GetKey()] = s
		for _, part := range stringParts(s) {
			if hasTelegramBrand(part) {
				t.Fatalf("telegram brand in new %s %q", s.GetKey(), part)
			}
		}
	}
	for _, key := range []string{"TellomiLoginWithPhone", "NotificationsInternal", "OutgoingCallsCount", "TellomiMapAmap"} {
		if seen[key] == nil {
			t.Fatalf("getDifference missed %s", key)
		}
	}
	if url, ok := seen["SearchEngine2SearchURL"]; ok && url.GetValue() != "https://duckduckgo.com/?q=" {
		t.Fatalf("search URL %q", url.GetValue())
	}
	phone := seen["TellomiLoginWithPhone"].GetValue()
	if phone != "使用手机号登录" {
		t.Fatalf("login %q", phone)
	}
	calls := seen["OutgoingCallsCount"]
	if calls.GetPredicateName() != mtproto.Predicate_langPackStringPluralized || calls.GetOtherValue() != "%1$d 通拨出电话" || calls.GetOneValue().GetValue() != "%1$d 通拨出电话" {
		t.Fatalf("call plural %+v", calls)
	}
	intro := seen["TellomiIntroPage1Message"].GetValue()
	if !strings.Contains(intro, "**私密**") || !strings.Contains(intro, "\n") {
		t.Fatalf("intro markup %q", intro)
	}
	caught, err := cat.Difference("android", "zh-hans", diff.GetVersion())
	if err != nil || len(caught.GetStrings()) != 0 || caught.GetVersion() != diff.GetVersion() {
		t.Fatalf("caught up %+v %v", caught, err)
	}
}

// TestTellomiKeysHaveNoHyphen guards iOS PresentationStrings generation.
// A hyphen in a Tellomi.* key is not a valid Swift identifier.
func TestTellomiKeysHaveNoHyphen(t *testing.T) {
	if err := DefaultError(); err != nil {
		t.Fatal(err)
	}
	cat := DefaultCatalog()
	var checked int
	for platform, langs := range cat.platforms {
		for code, lang := range langs {
			for _, e := range lang.meta.Strings {
				if !strings.HasPrefix(e.Key, "Tellomi.") {
					continue
				}
				checked++
				if strings.Contains(e.Key, "-") {
					t.Errorf("%s/%s key %s contains '-'", platform, code, e.Key)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no Tellomi keys in the catalog")
	}
	// Overrides and machine files are reapplied by scripts/langpack_official.py.
	// A hyphen there would return on the next import.
	for _, root := range []string{"../../data/langpack/overrides", "../../data/langpack/machine"} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".json.gz") {
				return nil
			}
			fh, err := os.Open(path)
			if err != nil {
				return err
			}
			defer fh.Close()
			zr, err := gzip.NewReader(fh)
			if err != nil {
				return err
			}
			defer zr.Close()
			var doc struct {
				Strings []struct {
					Key string `json:"key"`
				} `json:"strings"`
			}
			if err := json.NewDecoder(zr).Decode(&doc); err != nil {
				return err
			}
			for _, s := range doc.Strings {
				if strings.HasPrefix(s.Key, "Tellomi.") && strings.Contains(s.Key, "-") {
					t.Errorf("%s key %s contains '-'", path, s.Key)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestLangpackKeyNamesAreNotHosts fails when a host rewrite has been applied
// to a key. Chat.Message contains the letters t.me; rewriting that substring
// produced keys such as Chatell.ccssage. The check covers shipped packs and
// the override and machine catalogs the importer reapplies.
func TestLangpackKeyNamesAreNotHosts(t *testing.T) {
	root := filepath.Join("..", "..", "data", "langpack")
	var files, keys int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json.gz") {
			return nil
		}
		fh, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fh.Close()
		zr, err := gzip.NewReader(fh)
		if err != nil {
			return err
		}
		defer zr.Close()
		var doc struct {
			Strings []struct {
				Key string `json:"key"`
			} `json:"strings"`
		}
		if err := json.NewDecoder(zr).Decode(&doc); err != nil {
			return err
		}
		files++
		for _, s := range doc.Strings {
			keys++
			if strings.Contains(strings.ToLower(s.Key), "tell.cc") {
				t.Errorf("%s key %s was rewritten", path, s.Key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 || keys < 1000 {
		t.Fatalf("langpack scan files=%d keys=%d", files, keys)
	}
}

// TestIOSEnglishOverrideDoesNotPinHostKeys keeps the official Chat.Message
// strings out of the Tellomi override. A copy with the same text would become
// a divergence the next time the export changes, and the old wording would win.
func TestIOSEnglishOverrideDoesNotPinHostKeys(t *testing.T) {
	path := filepath.Join("..", "..", "data", "langpack", "overrides", "ios", "en.json.gz")
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
			Key string `json:"key"`
		} `json:"strings"`
	}
	if err := json.NewDecoder(zr).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	banned := map[string]bool{
		"Chat.Message.TopicAuthorBadge":    true,
		"Chatell.ccssage.TopicAuthorBadge": true,
		"Attachment.MediaAccessText":       true,
		"Attachmentell.ccdiaAccessText":    true,
	}
	for _, s := range doc.Strings {
		if banned[s.Key] {
			t.Fatalf("override pins %s", s.Key)
		}
	}
}
