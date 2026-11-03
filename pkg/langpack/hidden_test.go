package langpack

import (
	"os"
	"testing"
)

func TestHiddenLanguagesStayServed(t *testing.T) {
	prev, had := os.LookupEnv(hiddenLangEnv)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(hiddenLangEnv, prev)
		} else {
			_ = os.Unsetenv(hiddenLangEnv)
		}
	})

	if err := os.Unsetenv(hiddenLangEnv); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"hi", "th", "el", "HI", "th-TH", "el-GR"} {
		if !HiddenLang(code) {
			t.Fatalf("default should hide %s", code)
		}
	}
	if HiddenLang("fr") || HiddenLang("ja") || HiddenLang("zh-hans") {
		t.Fatal("default hid a full pack")
	}
	cat := DefaultCatalog()
	for _, platform := range []string{"ios", "android", "tdesktop", "macos"} {
		assertListed(t, cat, platform, false, "hi", "th", "el")
		for _, code := range []string{"hi", "th", "el"} {
			one, err := cat.Language(platform, code)
			if err != nil || one.GetLangCode() != code {
				t.Fatalf("%s %s language %+v %v", platform, code, one, err)
			}
			diff, err := cat.Difference(platform, code, 0)
			if err != nil || diff.GetLangCode() != code {
				t.Fatalf("%s %s pack %+v %v", platform, code, diff, err)
			}
			if _, err := cat.Strings(platform, code, []string{"AppName"}); err != nil {
				t.Fatalf("%s %s strings %v", platform, code, err)
			}
		}
	}

	if err := os.Setenv(hiddenLangEnv, ""); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"ios", "android", "tdesktop", "macos"} {
		assertListed(t, cat, platform, true, "hi", "th", "el")
	}
	if SuggestLangCode("en", "hi") != "hi" {
		t.Fatalf("empty hide list suggested %s", SuggestLangCode("en", "hi"))
	}

	if err := os.Setenv(hiddenLangEnv, "ja"); err != nil {
		t.Fatal(err)
	}
	assertListed(t, cat, "android", false, "ja")
	assertListed(t, cat, "ios", true, "hi", "th", "el")
	ja, err := cat.Language("android", "ja")
	if err != nil || ja.GetLangCode() != "ja" || ja.GetNativeName() != "日本語" {
		t.Fatalf("hidden ja still served %+v %v", ja, err)
	}
	if SuggestLangCode("en", "ja") != "en" || SuggestLangCode("fr", "ja") != "fr" {
		t.Fatalf("ja suggestion %s %s", SuggestLangCode("en", "ja"), SuggestLangCode("fr", "ja"))
	}
}

func assertListed(t *testing.T, cat *Catalog, platform string, want bool, codes ...string) {
	t.Helper()
	langs, err := cat.Languages(platform)
	if err != nil {
		t.Fatal(platform, err)
	}
	got := map[string]bool{}
	for _, lang := range langs {
		got[lang.GetLangCode()] = true
	}
	for _, code := range codes {
		if got[code] != want {
			t.Fatalf("%s listed %s = %v, want %v (%d languages)", platform, code, got[code], want, len(langs))
		}
	}
}
