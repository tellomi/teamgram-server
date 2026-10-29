package main

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
)

// fnLine matches "auth.sendCode#a677244f ... = auth.SentCode;"
var fnLine = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]*)\.([a-zA-Z][a-zA-Z0-9_]*)#([0-9a-fA-F]+)\b`)

// SchemaMethod is one TL function from the current layer.
type SchemaMethod struct {
	Method string
	CRC    string
}

// parseSchema reads TL functions after ---functions---.
func parseSchema(path string) (layer int, methods []SchemaMethod, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inFn := false
	seen := map[string]SchemaMethod{}
	layerRe := regexp.MustCompile(`^// LAYER (\d+)`)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := layerRe.FindStringSubmatch(line); m != nil {
			layer = atoi(m[1])
		}
		if line == "---functions---" {
			inFn = true
			continue
		}
		if !inFn || line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "---") {
			continue
		}
		m := fnLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1] + "." + m[2]
		// Keep the last declaration. Layer files occasionally repeat a name.
		seen[name] = SchemaMethod{Method: name, CRC: strings.ToLower(m[3])}
	}
	if err := sc.Err(); err != nil {
		return 0, nil, err
	}
	methods = make([]SchemaMethod, 0, len(seen))
	for _, m := range seen {
		methods = append(methods, m)
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Method < methods[j].Method })
	return layer, methods, nil
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// namespaces returns TL namespaces longest-first so "chatlists" wins over "chat".
func namespaces(methods []SchemaMethod) []string {
	set := map[string]struct{}{}
	for _, m := range methods {
		ns, _, _ := strings.Cut(m.Method, ".")
		set[ns] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for ns := range set {
		out = append(out, ns)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// canonicalGRPC turns "account_getAppConfig61E3F854" into "account.getAppConfig".
func canonicalGRPC(suffix string) string {
	ns, name, ok := strings.Cut(suffix, "_")
	if !ok || ns == "" || name == "" {
		return suffix
	}
	return ns + "." + stripConstructor(name)
}

// stripConstructor removes a trailing TL constructor id (6–8 hex digits).
func stripConstructor(name string) string {
	for n := 8; n >= 6; n-- {
		if len(name) <= n+1 {
			continue
		}
		suf := name[len(name)-n:]
		if !isHex(suf) || !hasDigit(suf) {
			continue
		}
		prev := name[len(name)-n-1]
		if prev >= '0' && prev <= '9' {
			continue
		}
		return name[:len(name)-n]
	}
	return name
}

func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return len(s) > 0
}

func hasDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}
