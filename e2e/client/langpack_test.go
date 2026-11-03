package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestLangPackBeforeLogin is what the login language picker does: the five
// langpack RPCs run on a fresh auth key, before auth.signIn.
func TestLangPackBeforeLogin(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)

	langs := invokeOK(t, c, &mtproto.TLLangpackGetLanguages{
		Constructor: mtproto.CRC32_langpack_getLanguages_42c6978f,
		LangPack:    "android",
	})
	vec, ok := langs.(*tlVector)
	if !ok || vec.Count < 22 || len(vec.Langs) != int(vec.Count) {
		t.Fatalf("langpack.getLanguages = %T %+v", langs, langs)
	}
	for _, lang := range vec.Langs {
		switch lang.GetLangCode() {
		case "hi", "th", "el":
			t.Fatalf("offered %s", lang.GetLangCode())
		}
	}

	// A client that already selected a hidden pack still receives it.
	hidden := invokeOK(t, c, &mtproto.TLLangpackGetLangPack{
		Constructor: mtproto.CRC32_langpack_getLangPack_f2f2330a,
		LangPack:    "android",
		LangCode:    "hi",
	})
	hiPack, ok := hidden.(*mtproto.TLLangPackDifference)
	if !ok || hiPack.GetLangCode() != "hi" {
		t.Fatalf("hidden hi pack = %T %+v", hidden, hidden)
	}

	// suggested_lang_code follows the device language, not the app language,
	// and does not suggest a hidden pack.
	french := ProfileAndroid
	french.SystemLangCode = "fr"
	french.LangCode = "en"
	c.SetProfile(french)
	cfgObj := invokeOK(t, c, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	got, ok := cfgObj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("help.getConfig = %T", cfgObj)
	}
	if got.GetSuggestedLangCode().GetValue() != "fr" {
		t.Fatalf("suggested from fr device %q", got.GetSuggestedLangCode().GetValue())
	}
	if got.GetLangPackVersion().GetValue() < 1 {
		t.Fatal("lang_pack_version missing")
	}
	hindi := ProfileAndroid
	hindi.SystemLangCode = "hi"
	hindi.LangCode = "ja"
	c.SetProfile(hindi)
	cfgObj = invokeOK(t, c, &mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	got, ok = cfgObj.(*mtproto.TLConfig)
	if !ok {
		t.Fatalf("help.getConfig = %T", cfgObj)
	}
	if got.GetSuggestedLangCode().GetValue() != "ja" {
		t.Fatalf("suggested from hi device %q", got.GetSuggestedLangCode().GetValue())
	}
	c.SetProfile(ProfileAndroid)

	one := invokeOK(t, c, &mtproto.TLLangpackGetLanguage{
		Constructor: mtproto.CRC32_langpack_getLanguage,
		LangPack:    "android",
		LangCode:    "ja",
	})
	lang, ok := one.(*mtproto.TLLangPackLanguage)
	if !ok {
		t.Fatalf("langpack.getLanguage = %T", one)
	}
	if lang.GetLangCode() != "ja" || lang.GetNativeName() != "日本語" || lang.GetTranslatedCount() < 1000 {
		t.Fatalf("ja language %+v", lang)
	}

	pack := invokeOK(t, c, &mtproto.TLLangpackGetLangPack{
		Constructor: mtproto.CRC32_langpack_getLangPack_f2f2330a,
		LangPack:    "android",
		LangCode:    "ja",
	})
	diff, ok := pack.(*mtproto.TLLangPackDifference)
	if !ok {
		t.Fatalf("langpack.getLangPack = %T", pack)
	}
	if diff.GetLangCode() != "ja" || diff.GetVersion() < 1 || len(diff.GetStrings()) < 1000 {
		t.Fatalf("ja pack code=%s version=%d strings=%d", diff.GetLangCode(), diff.GetVersion(), len(diff.GetStrings()))
	}
	var appName string
	for _, s := range diff.GetStrings() {
		if s.GetKey() == "AppName" {
			appName = s.GetValue()
			break
		}
		if s.GetKey() == "LanguageName" && s.GetValue() != "日本語" {
			t.Fatalf("LanguageName %q", s.GetValue())
		}
	}
	if appName != "Tellomi" {
		t.Fatalf("AppName %q", appName)
	}

	caught := invokeOK(t, c, &mtproto.TLLangpackGetDifference{
		Constructor: mtproto.CRC32_langpack_getDifference,
		LangPack:    "android",
		LangCode:    "ja",
		FromVersion: diff.GetVersion(),
	})
	empty, ok := caught.(*mtproto.TLLangPackDifference)
	if !ok || len(empty.GetStrings()) != 0 || empty.GetVersion() != diff.GetVersion() {
		t.Fatalf("getDifference = %T %+v", caught, caught)
	}

	c.SetProfile(ProfileIOS)
	iosLang := invokeOK(t, c, &mtproto.TLLangpackGetLanguage{
		Constructor: mtproto.CRC32_langpack_getLanguage,
		LangPack:    "ios",
		LangCode:    "zh-hant",
	})
	trad, ok := iosLang.(*mtproto.TLLangPackLanguage)
	if !ok || trad.GetLangCode() != "zh-hant" || trad.GetNativeName() != "繁體中文" || trad.GetTranslatedCount() < 1000 {
		t.Fatalf("zh-hant %+v", iosLang)
	}

	// A client that still has a removed duplicate selected must get the
	// official pack, not an error and not a second Chinese entry.
	removed := invokeOK(t, c, &mtproto.TLLangpackGetLangPack{
		Constructor: mtproto.CRC32_langpack_getLangPack_f2f2330a,
		LangPack:    "ios",
		LangCode:    "classic-zh-cn",
	})
	hans, ok := removed.(*mtproto.TLLangPackDifference)
	if !ok || hans.GetLangCode() != "zh-hans" || len(hans.GetStrings()) < 1000 {
		t.Fatalf("classic-zh-cn fallback %+v", removed)
	}
	twLang := invokeOK(t, c, &mtproto.TLLangpackGetLanguage{
		Constructor: mtproto.CRC32_langpack_getLanguage,
		LangPack:    "ios",
		LangCode:    "taiwan",
	})
	tw, ok := twLang.(*mtproto.TLLangPackLanguage)
	if !ok || tw.GetLangCode() != "zh-hant" || tw.GetNativeName() != "繁體中文" {
		t.Fatalf("taiwan fallback %+v", twLang)
	}
}
