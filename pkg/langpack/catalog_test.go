package langpack

import (
	"regexp"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var placeholderRe = regexp.MustCompile(`https?://[^\s<>()]+|t\.me/[^\s<>()]+|%%|%(?:\d+\$)?\d*[dfsiu@]|%(?:\d+\$)?\.\d+f|\{[A-Za-z0-9_]+\}|\bun\d+\b`)

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
		if len(langs) != 3 {
			t.Fatalf("%s languages %d", platform, len(langs))
		}
		byCode := map[string]*mtproto.LangPackLanguage{}
		for _, lang := range langs {
			byCode[lang.GetLangCode()] = lang
			if lang.GetNativeName() == "" || lang.GetPluralCode() == "" || lang.GetTranslationsUrl() == "" || !lang.GetOfficial() {
				t.Fatalf("%s incomplete %+v", platform, lang)
			}
			if lang.GetStringsCount() < 1000 || lang.GetTranslatedCount() < 1000 {
				t.Fatalf("%s %s counts %d/%d", platform, lang.GetLangCode(), lang.GetTranslatedCount(), lang.GetStringsCount())
			}
			diff, err := cat.Difference(platform, lang.GetLangCode(), 0)
			if err != nil || diff.GetVersion() < 1 || len(diff.GetStrings()) < 1000 {
				t.Fatalf("%s %s diff %v %v", platform, lang.GetLangCode(), diff.GetVersion(), err)
			}
			buf := mtproto.NewEncodeBuf(256)
			if err := lang.Encode(buf, 232); err != nil {
				t.Fatal(err)
			}
			if err := diff.GetStrings()[0].Encode(buf, 232); err != nil {
				t.Fatal(err)
			}
			for _, s := range diff.GetStrings() {
				if hasTelegramBrand(s.GetValue()) || hasTelegramBrand(s.GetOtherValue()) {
					t.Fatalf("brand in %s %s %q", platform, s.GetKey(), s.GetValue()+s.GetOtherValue())
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
	if android[0].GetValue() != "Tellomi" || android[1].GetValue() != "设置" || android[2].GetValue() != "选择语言" {
		t.Fatalf("android alert strings %+v", android)
	}
	ios, err := cat.Strings("ios", "zh-hant", []string{"Settings.Title"})
	if err != nil || ios[0].GetValue() != "設定" {
		t.Fatalf("ios traditional %+v %v", ios, err)
	}

	// Placeholders in the English pack must survive in both Chinese packs.
	for _, platform := range []string{"ios", "android"} {
		en, err := cat.Difference(platform, "en", 0)
		if err != nil {
			t.Fatal(err)
		}
		zh, err := cat.Difference(platform, "zh-hans", 0)
		if err != nil {
			t.Fatal(err)
		}
		enTok := tokensByKey(en.GetStrings())
		zhTok := tokensByKey(zh.GetStrings())
		if len(enTok) == 0 || len(enTok) != len(zhTok) {
			t.Fatalf("%s key mismatch %d %d", platform, len(enTok), len(zhTok))
		}
		for key, toks := range enTok {
			if strings.Join(toks, "\n") != strings.Join(zhTok[key], "\n") {
				t.Fatalf("%s placeholder drift %s\nen %q\nzh %q", platform, key, toks, zhTok[key])
			}
		}
	}
}

func tokensByKey(stringsIn []*mtproto.LangPackString) map[string][]string {
	out := map[string][]string{}
	for _, s := range stringsIn {
		parts := []string{s.GetValue(), s.GetOtherValue()}
		for _, opt := range []*wrapperspb.StringValue{s.GetZeroValue(), s.GetOneValue(), s.GetTwoValue(), s.GetFewValue(), s.GetManyValue()} {
			if opt != nil {
				parts = append(parts, opt.Value)
			}
		}
		var toks []string
		for _, part := range parts {
			toks = append(toks, placeholderTokens(part)...)
		}
		out[s.GetKey()] = append(out[s.GetKey()], toks...)
	}
	return out
}

func placeholderTokens(s string) []string {
	return placeholderRe.FindAllString(s, -1)
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
		if len(langs) != 3 {
			t.Fatalf("%s languages %d", platform, len(langs))
		}
		for _, lang := range langs {
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
