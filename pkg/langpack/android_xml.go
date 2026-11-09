package langpack

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// androidKeyRE is the name stock Android writes into <string name="...">.
// A key outside this set breaks the XML file or is not a string name the
// client can read back.
var androidKeyRE = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// androidEntrySafe reports whether stock Android 13.1.1 can store the entry.
// saveRemoteLocaleStrings writes every string with escapeString, which only
// rewrites "<", ">" and "& " (ampersand followed by a space) and leaves a
// value untouched when it contains "[CDATA". The file is then read with
// XmlPullParser. One bad value makes that parse fail, the client throws the
// map away, and the whole language falls back to English.
//
// Values are dropped, not rewritten to "&amp;". The merge path reads the
// file back with preserveEscapes and only restores "& ", so an escaped
// ampersand would be written bare again on the next difference.
//
// A well-formed character reference such as "&#160;" is kept. XmlPullParser
// accepts it, and the official packs ship those references. A bare "&q=" is
// not an entity and is dropped.
func androidEntrySafe(e Entry) bool {
	if !androidKeyRE.MatchString(e.Key) {
		return false
	}
	if !androidValueSafe(e.Value) {
		return false
	}
	if e.Plural == nil {
		return true
	}
	for _, part := range []string{e.Plural.Zero, e.Plural.One, e.Plural.Two, e.Plural.Few, e.Plural.Many, e.Plural.Other} {
		if !androidValueSafe(part) {
			return false
		}
	}
	return true
}

func androidValueSafe(s string) bool {
	if s == "" {
		return true
	}
	if strings.IndexFunc(s, androidIllegalControl) >= 0 {
		return false
	}
	if strings.Contains(s, "[CDATA") && !androidCDATAWellFormed(s) {
		return false
	}
	if !strings.ContainsAny(s, "&<>") && !strings.Contains(s, "[CDATA") {
		return true
	}
	return androidEscapedTextWellFormed(androidEscapeString(s))
}

// androidIllegalControl is an XML 1.0 control character. Tab, LF and CR stay.
func androidIllegalControl(r rune) bool {
	return r <= 0x08 || r == 0x0B || r == 0x0C || (r >= 0x0E && r <= 0x1F)
}

// androidCDATAWellFormed reports whether every "[CDATA" opens a
// "<![CDATA[...]]>" section. escapeString copies such a value through
// unchanged, so a space inside the marker ("<! [CDATA [") is written raw
// and the XML parser rejects the file.
func androidCDATAWellFormed(s string) bool {
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], "[CDATA")
		if j < 0 {
			return true
		}
		j += i
		if j >= 2 && strings.HasPrefix(s[j-2:], "<![CDATA[") {
			end := strings.Index(s[j+7:], "]]>")
			if end < 0 {
				return false
			}
			i = j + 7 + end + 3
			continue
		}
		return false
	}
	return true
}

// androidEscapeString is stock Android LocaleController.escapeString from the
// 13.1.1 client (Telegram PR #111): "<", ">", and "& " only, unless the
// value contains "[CDATA", in which case it is copied unchanged.
func androidEscapeString(s string) string {
	if strings.Contains(s, "[CDATA") {
		return s
	}
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "& ", "&amp; ")
	return s
}

func androidEscapedTextWellFormed(escaped string) bool {
	var buf bytes.Buffer
	buf.Grow(len(escaped) + 64)
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?><resources><string name="k">`)
	buf.WriteString(escaped)
	buf.WriteString(`</string></resources>`)
	dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
		if ch, ok := tok.(xml.CharData); ok && !xmlCharDataOK(ch) {
			return false
		}
	}
}

func xmlCharDataOK(ch xml.CharData) bool {
	s := string(ch)
	for _, r := range s {
		if r == utf8.RuneError && !utf8.ValidString(s) {
			return false
		}
		if !xml10Char(r) {
			return false
		}
	}
	return true
}

// xml10Char is the XML 1.0 Char production. encoding/xml rejects the same set.
func xml10Char(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}
