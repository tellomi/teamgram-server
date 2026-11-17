package langpack

import "testing"

func TestClientLangOrder(t *testing.T) {
	if got := ClientLang("zh-hans", "en", "android"); got != "zh-hans" {
		t.Fatal(got)
	}
	if got := ClientLang("en", "zh-Hans", "android"); got != "en" {
		t.Fatal(got)
	}
	if got := ClientLang("", "zh-CN", "android"); got != "zh-hans" {
		t.Fatal(got)
	}
	if got := ClientLang("", "", "zh-hant"); got != "zh-hant" {
		t.Fatal(got)
	}
	// lang_pack is tried before system_lang_code. A platform name is skipped.
	if got := ClientLang("", "en", "zh-hant"); got != "zh-hant" {
		t.Fatalf("lang_pack before system: %s", got)
	}
	if got := ClientLang("", "zh-Hant-HK", "android"); got != "zh-hant" {
		t.Fatalf("system after a platform pack: %s", got)
	}
	if got := ClientLang("ja-JP", "en", "android"); got != "ja" {
		t.Fatalf("ja-JP: %s", got)
	}
	if got := ClientLang("zz", "en-US", "zh-TW"); got != "zh-hant" {
		t.Fatalf("unknown lang_code then lang_pack: %s", got)
	}
	if got := ClientLang("android", "ios", "tdesktop"); got != "en" {
		t.Fatal(got)
	}
	if got := ClientLang("classic-zh-cn", "", ""); got != "zh-hans" {
		t.Fatal(got)
	}
	if got := ClientLang("pt_BR", "", ""); got != "pt-br" {
		t.Fatal(got)
	}
}

func TestCanonicalAndSuggest(t *testing.T) {
	t.Setenv(hiddenLangEnv, defaultHiddenLangs)
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"zh", "zh-hans", true},
		{"zh-CN", "zh-hans", true},
		{"zh_CN", "zh-hans", true},
		{"zh-Hans", "zh-hans", true},
		{"zh-Hans-CN", "zh-hans", true},
		{"zh-Hant", "zh-hant", true},
		{"zh-Hant-TW", "zh-hant", true},
		{"zh-TW", "zh-hant", true},
		{"zh_HK", "zh-hant", true},
		{"zh-MO", "zh-hant", true},
		{"zh-rTW", "zh-hant", true},
		{"zh-rHK", "zh-hant", true},
		{"zh-rMO", "zh-hant", true},
		{"zh-rCN", "zh-hans", true},
		{"zh-rSG", "zh-hans", true},
		{"zh-cht", "zh-hant", true},
		{"zh-CHS", "zh-hans", true},
		{"cmn-Hans", "zh-hans", true},
		{"cmn-Hant", "zh-hant", true},
		{"yue", "zh-hant", true},
		{"yue-HK", "zh-hant", true},
		{"zh-Latn", "", false},
		{"ja-JP", "ja", true},
		{"ko-KR", "ko", true},
		{"de_DE", "de", true},
		{"fr-FR", "fr", true},
		{"in", "id", true},
		{"in-ID", "id", true},
		{"iw-IL", "he", true},
		{"TW", "zh-hant", true},
		{"HK", "zh-hant", true},
		{"en", "en", true},
		{"en-US", "en", true},
		{"fr", "fr", true},
		{"classic-zh-cn", "zh-hans", true},
		{"classic_zh_cn", "zh-hans", true},
		{"taiwan", "zh-hant", true},
		{"Taiwan", "zh-hant", true},
		{"th", "th", true},
		{"hi", "hi", true},
		{"pt_BR", "pt-br", true},
		{"iw", "he", true},
		{"no", "nb", true},
		{"no-NO", "nb", true},
		{"no-such", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := CanonicalLang(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("CanonicalLang(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}

	suggests := []struct {
		lang, system, want string
	}{
		{"zh", "", "zh-hans"},
		{"en", "zh-Hant", "zh-hant"},
		{"en", "zh-TW", "zh-hant"},
		{"en", "zh-HK", "zh-hant"},
		{"en", "zh-Hant-HK", "zh-hant"},
		{"en", "zh-rTW", "zh-hant"},
		{"en", "zh-cht", "zh-hant"},
		{"en", "yue-HK", "zh-hant"},
		{"en", "cmn-Hant", "zh-hant"},
		{"en", "cmn-Hans", "zh-hans"},
		{"en", "zh-chs", "zh-hans"},
		{"zh-hans", "zh-TW", "zh-hant"},
		{"zh-HK", "en", "en"},
		{"zh_CN", "en-US", "en"},
		{"en", "en-US", "en"},
		{"", "", "en"},
		{"fr", "de", "de"},
		{"xx", "de", "de"},
		{"en", "fr", "fr"},
		{"en", "fr-FR", "fr"},
		{"en", "ja-JP", "ja"},
		{"en", "de_DE", "de"},
		{"en", "pt-BR", "pt-br"},
		{"de", "", "de"},
		{"hi", "", "en"},
		{"en", "hi", "en"},
		{"en", "hi-IN", "en"},
		{"ja", "th", "ja"},
		{"ja", "el-GR", "ja"},
		{"el", "el", "en"},
		{"zh", "el", "zh-hans"},
		{"xx", "yy", "en"},
		{"en", "pt-PT", "en"},
	}
	for _, tc := range suggests {
		if got := SuggestLangCode(tc.lang, tc.system); got != tc.want {
			t.Errorf("SuggestLangCode(%q, %q) = %q; want %q", tc.lang, tc.system, got, tc.want)
		}
	}
}

func TestNormalizePlatform(t *testing.T) {
	if NormalizePlatform("android_x") != "android" || NormalizePlatform("macOS") != "macos" || NormalizePlatform("tdesktop") != "tdesktop" {
		t.Fatal(NormalizePlatform("android_x"), NormalizePlatform("macOS"), NormalizePlatform("tdesktop"))
	}
}
