// Package emojiblock is the server emoji blocklist published in
// help.getAppConfig and applied to sticker, custom-emoji, and reaction
// payloads. The Taiwan flag is always included. Environment and an optional
// file append more entries. A process reads the config at startup; tests
// call Reload after changing the environment.
package emojiblock

import (
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/text/unicode/norm"
)

const (
	// EnvExtra appends emoji to the built-in blocklist.
	// Entries are separated by commas or newlines. Each entry is a literal
	// emoji or a code-point sequence such as "U+1F1F9 U+1F1FC" or
	// "1F1F9-1F1FC". A code-point sequence is one entry: commas split entries,
	// spaces and hyphens split code points inside an entry.
	EnvExtra = "TELLOMI_EMOJI_BLOCKLIST"
	// EnvFile is an optional UTF-8 file. One entry per line, same syntax as
	// EnvExtra. Blank lines and lines starting with # are ignored. A missing
	// file is logged and skipped; the built-in list still applies.
	EnvFile = "TELLOMI_EMOJI_BLOCKLIST_FILE"
)

// TaiwanFlag is regional indicators T + W, U+1F1F9 U+1F1FC.
const TaiwanFlag = "\U0001F1F9\U0001F1FC"

type snapshot struct {
	list []string
	set  map[string]struct{}
}

var (
	current atomic.Value
	builtin = snapshot{}
)

func init() {
	builtin = compile([]string{TaiwanFlag})
	current.Store(load())
}

// Reload reads the environment again.
func Reload() {
	current.Store(load())
}

// List is the blocklist in config order. The Taiwan flag is first.
// The returned slice is a copy.
func List() []string {
	snap := current.Load().(snapshot)
	out := make([]string, len(snap.list))
	copy(out, snap.list)
	return out
}

// Blocked reports whether emoji is on the blocklist.
// U+FE0E, U+FE0F, and zero-width characters other than ZWJ are ignored, so
// a presentation variant matches the same entry.
func Blocked(emoji string) bool {
	key := normalize(emoji)
	if key == "" {
		return false
	}
	snap := current.Load().(snapshot)
	_, ok := snap.set[key]
	return ok
}

// Fingerprint identifies the current list. It changes when an entry is added.
func Fingerprint() int64 {
	return fingerprint(current.Load().(snapshot).list)
}

// BuiltinFingerprint identifies the built-in list, ignoring env and file.
func BuiltinFingerprint() int64 {
	return fingerprint(builtin.list)
}

func fingerprint(list []string) int64 {
	h := fnv.New64a()
	for _, emo := range list {
		_, _ = h.Write([]byte(emo))
		_, _ = h.Write([]byte{0})
	}
	out := int64(h.Sum64() & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}

func load() snapshot {
	items := []string{TaiwanFlag}
	items = append(items, parseEntries(os.Getenv(EnvExtra))...)
	if path := strings.TrimSpace(os.Getenv(EnvFile)); path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			logx.Errorf("emoji blocklist file %s: %v", path, err)
		} else {
			items = append(items, parseEntries(string(body))...)
		}
	}
	return compile(items)
}

func compile(items []string) snapshot {
	snap := snapshot{set: map[string]struct{}{}}
	for _, item := range items {
		item = normalize(item)
		if item == "" {
			continue
		}
		if _, ok := snap.set[item]; ok {
			continue
		}
		snap.set[item] = struct{}{}
		snap.list = append(snap.list, item)
	}
	if len(snap.list) == 0 {
		snap.set[TaiwanFlag] = struct{}{}
		snap.list = []string{TaiwanFlag}
	}
	return snap
}

func parseEntries(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		for _, part := range strings.Split(line, ",") {
			item, ok := parseEntry(part)
			if ok {
				out = append(out, item)
			}
		}
	}
	return out
}

func parseEntry(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if decoded, ok := parseCodepoints(raw); ok {
		raw = decoded
	}
	s := normalize(raw)
	if s == "" {
		return "", false
	}
	return s, true
}

// parseCodepoints accepts "U+1F1F9 U+1F1FC", "1F1F9-1F1FC", and "1F1F9_1F1FC".
// A value that is not entirely code points returns false so a literal emoji
// is kept as text.
func parseCodepoints(raw string) (string, bool) {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '-' || r == '_'
	})
	if len(parts) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, part := range parts {
		hex := part
		if len(hex) > 2 && (hex[0] == 'U' || hex[0] == 'u') && hex[1] == '+' {
			hex = hex[2:]
		}
		if !isHexCode(hex) {
			return "", false
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil || v == 0 || v > unicode.MaxRune || !utf8.ValidRune(rune(v)) {
			return "", false
		}
		b.WriteRune(rune(v))
	}
	return b.String(), true
}

func isHexCode(s string) bool {
	if len(s) < 4 || len(s) > 6 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// normalize strips presentation selectors and non-ZWJ zero-width characters
// and composes to NFC. ZWJ sequences stay intact.
func normalize(s string) string {
	s = norm.NFC.String(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 0xFE0E, 0xFE0F, 0xFEFF, 0x200B, 0x200C, 0x2060:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
