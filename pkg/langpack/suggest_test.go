package langpack

import "testing"

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
