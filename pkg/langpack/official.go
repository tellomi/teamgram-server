package langpack

// OfficialLanguage is one language served from langpack.getLanguages.
// String bodies come from the public translations.telegram.org export.
// See docs/languages.md.
type OfficialLanguage struct {
	Code       string
	Name       string
	NativeName string
	PluralCode string
	RTL        bool
}

// OfficialLanguages is every pack shipped under data/langpack.
// langpack.getLanguages omits TELLOMI_LANGPACK_HIDDEN (default hi, th, el).
// Simplified Chinese, Traditional Chinese, and English stay first in languageOrder.
// classic-zh-cn and taiwan are not listed: each repeated a Chinese pack
// (简体中文 next to zh-hans, 正體中文 next to zh-hant). CanonicalLang serves
// those old codes from zh-hans and zh-hant. BCP-47 zh-TW / zh-HK still
// resolve to zh-hant.
var OfficialLanguages = []OfficialLanguage{
	{Code: "en", Name: "English", NativeName: "English", PluralCode: "en"},
	{Code: "ar", Name: "Arabic", NativeName: "العربية", PluralCode: "ar", RTL: true},
	{Code: "be", Name: "Belarusian", NativeName: "Беларуская", PluralCode: "be"},
	{Code: "ca", Name: "Catalan", NativeName: "Català", PluralCode: "ca"},
	{Code: "cs", Name: "Czech", NativeName: "Čeština", PluralCode: "cs"},
	{Code: "de", Name: "German", NativeName: "Deutsch", PluralCode: "de"},
	{Code: "el", Name: "Greek", NativeName: "Ελληνικά", PluralCode: "el"},
	{Code: "es", Name: "Spanish", NativeName: "Español", PluralCode: "es"},
	{Code: "fa", Name: "Persian", NativeName: "فارسی", PluralCode: "fa", RTL: true},
	{Code: "fi", Name: "Finnish", NativeName: "Suomi", PluralCode: "fi"},
	{Code: "fr", Name: "French", NativeName: "Français", PluralCode: "fr"},
	{Code: "he", Name: "Hebrew", NativeName: "עברית", PluralCode: "he", RTL: true},
	{Code: "hi", Name: "Hindi", NativeName: "हिन्दी", PluralCode: "hi"},
	{Code: "id", Name: "Indonesian", NativeName: "Bahasa Indonesia", PluralCode: "id"},
	{Code: "it", Name: "Italian", NativeName: "Italiano", PluralCode: "it"},
	{Code: "ja", Name: "Japanese", NativeName: "日本語", PluralCode: "ja"},
	{Code: "ko", Name: "Korean", NativeName: "한국어", PluralCode: "ko"},
	{Code: "ms", Name: "Malay", NativeName: "Bahasa Melayu", PluralCode: "ms"},
	{Code: "nb", Name: "Norwegian", NativeName: "Norsk", PluralCode: "nb"},
	{Code: "nl", Name: "Dutch", NativeName: "Nederlands", PluralCode: "nl"},
	{Code: "pl", Name: "Polish", NativeName: "Polski", PluralCode: "pl"},
	{Code: "pt-br", Name: "Portuguese (Brazil)", NativeName: "Português (Brasil)", PluralCode: "pt"},
	{Code: "ro", Name: "Romanian", NativeName: "Română", PluralCode: "ro"},
	{Code: "ru", Name: "Russian", NativeName: "Русский", PluralCode: "ru"},
	{Code: "sv", Name: "Swedish", NativeName: "Svenska", PluralCode: "sv"},
	{Code: "th", Name: "Thai", NativeName: "ไทย", PluralCode: "th"},
	{Code: "tr", Name: "Turkish", NativeName: "Türkçe", PluralCode: "tr"},
	{Code: "uk", Name: "Ukrainian", NativeName: "Українська", PluralCode: "uk"},
	{Code: "uz", Name: "Uzbek", NativeName: "Oʻzbek", PluralCode: "uz"},
	{Code: "vi", Name: "Vietnamese", NativeName: "Tiếng Việt", PluralCode: "vi"},
	{Code: "zh-hans", Name: "Chinese (Simplified)", NativeName: "简体中文", PluralCode: "zh"},
	{Code: "zh-hant", Name: "Chinese (Traditional)", NativeName: "繁體中文", PluralCode: "zh"},
}

func officialCode(s string) (string, bool) {
	switch s {
	case "pt_br", "pt-br":
		return "pt-br", true
	case "iw":
		return "he", true
	case "no", "nb":
		return "nb", true
	case "in":
		// Android used to send in for Indonesian.
		return "id", true
	}
	for _, lang := range OfficialLanguages {
		if lang.Code == s {
			return lang.Code, true
		}
	}
	return "", false
}
