package catalog

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/width"
)

// folder applies Unicode case folding. It is safe to share.
var folder = cases.Fold()

// foldText makes title and keyword search ignore case and fullwidth forms.
// "Ｆｌｕｅｎｔ" and "fluent" fold to the same string. Han characters are unchanged.
func foldText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return folder.String(width.Fold.String(s))
}

// stripVS drops emoji variation selectors so "❤" and "❤️" compare equal.
func stripVS(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 0xFE0E || r == 0xFE0F {
			return -1
		}
		return r
	}, s)
}

func emojiEqual(a, b string) bool {
	a = stripVS(strings.TrimSpace(a))
	b = stripVS(strings.TrimSpace(b))
	return a != "" && a == b
}

// emojiListed reports whether emoticon occurs in a concatenation of emoji.
// Android messages.searchStickers sends that concatenation in emoticon
// after a local keyword hit.
func emojiListed(emoticon, haystack string) bool {
	needle := stripVS(strings.TrimSpace(emoticon))
	haystack = stripVS(haystack)
	if needle == "" || haystack == "" {
		return false
	}
	return strings.Contains(haystack, needle)
}

func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// canonicalEmojiLang maps the codes official clients send onto one list.
// Android getCurrentKeyboardLanguage uses the IME language tag with '_'
// turned into '-': zh-CN, zh-Hans, zh. iOS and the Tellomi langpack use
// zh-hans. Desktop asks messages.getEmojiKeywordsLanguages with the same
// strings and then fetches keywords for whatever code we return.
func canonicalEmojiLang(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	c = strings.ReplaceAll(c, "_", "-")
	if c == "" {
		return ""
	}
	switch c {
	case "zh", "zh-cn", "zh-sg", "zh-hans", "zh-cmn", "zh-cmn-hans", "zh-hans-cn", "zh-hans-sg", "chinese":
		return "zh-hans"
	case "zh-tw", "zh-hk", "zh-mo", "zh-hant", "zh-hant-tw", "zh-hant-hk", "zh-hant-mo":
		return "zh-hant"
	case "en", "en-us", "en-gb", "en-au", "en-ca":
		return "en"
	}
	switch {
	case strings.HasPrefix(c, "zh-hans"):
		return "zh-hans"
	case strings.HasPrefix(c, "zh-hant"), strings.HasPrefix(c, "zh-tw"), strings.HasPrefix(c, "zh-hk"):
		return "zh-hant"
	case c == "en" || strings.HasPrefix(c, "en-"):
		return "en"
	}
	if i := strings.IndexByte(c, '-'); i > 0 {
		return c[:i]
	}
	return c
}
