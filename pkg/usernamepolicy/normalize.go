package usernamepolicy

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// HasDisallowedControl reports Cc/Cf characters that must not be stripped
// into a different name: C0/C1 controls, zero-width and bidi marks, BOM,
// soft hyphen, and tag characters.
func HasDisallowedControl(input string) bool {
	for _, c := range input {
		switch {
		case c <= 0x1F || (c >= 0x7F && c <= 0x9F):
			return true
		case c == 0xAD || c == 0xFEFF || (c >= 0xFFF9 && c <= 0xFFFB):
			return true
		case c >= 0x200B && c <= 0x200F:
			return true
		case c >= 0x202A && c <= 0x202E:
			return true
		case c >= 0x2060 && c <= 0x2064:
			return true
		case c >= 0x2066 && c <= 0x2069:
			return true
		case c >= 0xE0000 && c <= 0xE007F:
			return true
		}
	}
	return false
}

// Normalize applies NFKC, Unicode lowercase, ß→ss, and whitespace collapse.
// The order matches the policy engine: a second implementation will drift.
func Normalize(input string) string {
	folded := strings.ToLower(norm.NFKC.String(input))
	// strings.ToLower uses simple mappings. İ becomes i; ß stays ß.
	folded = strings.ReplaceAll(folded, "\u00DF", "ss")

	var b strings.Builder
	b.Grow(len(folded))
	pending := false
	for _, c := range folded {
		if unicode.IsSpace(c) {
			pending = b.Len() > 0
			continue
		}
		if pending {
			b.WriteByte(' ')
			pending = false
		}
		b.WriteRune(c)
	}
	return b.String()
}

// Skeleton is the confusable prototype of an already-normalized string,
// then the ASCII lookalike fold used for usernames (1/l/i, 0/o, rn/m,
// separators dropped).
func Skeleton(normalized string) string {
	return asciiFold(confusablePrototype(normalized))
}

func asciiFold(s string) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(rs))
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '1', 'l', '|', 'i', '!':
			b.WriteByte('l')
		case '0':
			b.WriteByte('o')
		case '5', '$':
			b.WriteByte('s')
		case '3':
			b.WriteByte('e')
		case '4', '@':
			b.WriteByte('a')
		case '7':
			b.WriteByte('t')
		case 'r':
			if i+1 < len(rs) && rs[i+1] == 'n' {
				i++
				b.WriteByte('m')
				continue
			}
			b.WriteByte('r')
		case '_', '-', '.', ' ':
		default:
			b.WriteRune(rs[i])
		}
	}
	return b.String()
}

type scriptKind int

const (
	scriptCommon scriptKind = iota
	scriptLatin
	scriptCyrillic
	scriptGreek
	scriptHan
	scriptHiragana
	scriptKatakana
	scriptHangul
	scriptBopomofo
	scriptOther
)

// MixedScript is true when a word mixes scripts in a way that is not a
// single script and not Latin plus Han/Kana/Hangul/Bopomofo.
// Spaces split words first: "Bob 李" is two ordinary words.
func MixedScript(normalized string) bool {
	for _, word := range strings.Split(normalized, " ") {
		if word != "" && suspiciousWord(word) {
			return true
		}
	}
	return false
}

func suspiciousWord(word string) bool {
	seen := map[scriptKind]struct{}{}
	for _, r := range word {
		sc := scriptOf(r)
		if sc == scriptCommon {
			continue
		}
		seen[sc] = struct{}{}
	}
	if len(seen) <= 1 {
		return false
	}
	if _, latin := seen[scriptLatin]; latin && len(seen) == 2 {
		for sc := range seen {
			if sc == scriptLatin {
				continue
			}
			switch sc {
			case scriptHan, scriptHiragana, scriptKatakana, scriptHangul, scriptBopomofo:
				return false
			}
		}
	}
	return true
}

func scriptOf(r rune) scriptKind {
	switch {
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r):
		return scriptCommon
	case unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r):
		return scriptCommon
	case r <= 0x024F || (r >= 0x1E00 && r <= 0x1EFF) || (r >= 0x2C60 && r <= 0x2C7F) || (r >= 0xA720 && r <= 0xA7FF):
		return scriptLatin
	case (r >= 0x0400 && r <= 0x052F) || (r >= 0x2DE0 && r <= 0x2DFF) || (r >= 0xA640 && r <= 0xA69F):
		return scriptCyrillic
	case (r >= 0x0370 && r <= 0x03FF) || (r >= 0x1F00 && r <= 0x1FFF):
		return scriptGreek
	case (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0xF900 && r <= 0xFAFF):
		return scriptHan
	case r >= 0x3040 && r <= 0x309F:
		return scriptHiragana
	case r >= 0x30A0 && r <= 0x30FF:
		return scriptKatakana
	case (r >= 0x1100 && r <= 0x11FF) || (r >= 0xAC00 && r <= 0xD7AF):
		return scriptHangul
	case r >= 0x3100 && r <= 0x312F:
		return scriptBopomofo
	default:
		if r < 0x80 {
			return scriptLatin
		}
		return scriptOther
	}
}
