package usernamepolicy

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// Unicode Security Mechanisms confusables (UTS #39), version 16.0.0.
// License: https://www.unicode.org/terms_of_use.html
//
//go:embed data/confusables.txt
var confusablesTxt string

var (
	confusableOnce sync.Once
	confusableMap  map[rune]string
)

func confusablePrototype(s string) string {
	confusableOnce.Do(func() {
		confusableMap = parseConfusables(confusablesTxt)
	})
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if repl, ok := confusableMap[r]; ok {
			b.WriteString(repl)
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFD.String(b.String())
}

func parseConfusables(text string) map[rune]string {
	out := make(map[rune]string)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) < 2 {
			continue
		}
		src := hexRunes(parts[0])
		dst := hexRunes(parts[1])
		if len(src) != 1 || len(dst) == 0 {
			continue
		}
		out[src[0]] = string(dst)
	}
	return out
}

func hexRunes(s string) []rune {
	fields := strings.Fields(s)
	out := make([]rune, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "#") {
			break
		}
		v, err := strconv.ParseUint(f, 16, 32)
		if err != nil {
			return nil
		}
		out = append(out, rune(v))
	}
	return out
}
