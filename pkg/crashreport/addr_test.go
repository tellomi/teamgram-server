package crashreport

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestListenErrTextKeepsBindErrorAndMasksDSN(t *testing.T) {
	bind := errors.New("listen tcp 127.0.0.1:8819: bind: address already in use")
	if got := listenErrText(bind); got != bind.Error() {
		t.Fatalf("bind text %q", got)
	}
	secret := errors.New("dial tcp: teamgram:s3cret-db@tcp(mysql:3306)/teamgram")
	got := listenErrText(secret)
	if strings.Contains(got, "s3cret") {
		t.Fatalf("secret leaked: %s", got)
	}
	if !strings.Contains(got, "teamgram:***@tcp(mysql:3306)/teamgram") {
		t.Fatalf("dsn not masked: %s", got)
	}
	if listenErrText(nil) != "" {
		t.Fatal("nil error produced text")
	}
}

func TestDefaultAddrMissesEtc2Listeners(t *testing.T) {
	_, port, err := net.SplitHostPort(defaultAddr)
	if err != nil {
		t.Fatal(err)
	}
	if !AddrAllowed(defaultAddr) {
		t.Fatalf("default %s is not loopback", defaultAddr)
	}
	dir := filepath.Join("..", "..", "teamgramd", "etc2")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	hits := map[string]string{}
	files := 0
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		files++
		raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for p, where := range listenerPorts(ent.Name(), string(raw)) {
			addPort(hits, p, where)
		}
	}
	if files == 0 {
		t.Fatal("no bundled etc2 yaml")
	}
	for _, known := range []string{"20010", "10443", "8801", "11701", "3478"} {
		if _, ok := hits[known]; !ok {
			t.Fatalf("parser missed bundled listener port %s in %d files", known, files)
		}
	}
	if where, ok := hits[port]; ok {
		t.Fatalf("default %s uses port %s configured by %s", defaultAddr, port, where)
	}
}

func listenerPorts(filename, text string) map[string]string {
	out := map[string]string{}
	addresses := false
	for i, line := range strings.Split(text, "\n") {
		line = stripYAMLComment(line)
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		where := fmt.Sprintf("%s:%d", filename, i+1)
		if addresses && strings.HasPrefix(trim, "-") {
			item := strings.TrimSpace(strings.TrimPrefix(trim, "-"))
			if p, ok := hostPort(item); ok {
				addPort(out, p, where)
			}
			continue
		}
		addresses = false
		key, val, ok := yamlKey(trim)
		if !ok {
			continue
		}
		switch key {
		case "ListenOn", "Listen":
			if p, ok := hostPort(val); ok {
				addPort(out, p, where)
			}
		case "Addresses":
			if p, ok := hostPort(val); ok {
				addPort(out, p, where)
			}
			addresses = true
		default:
			if key == "Port" || strings.HasSuffix(key, "Port") {
				if p, ok := plainPort(val); ok {
					addPort(out, p, where)
				}
			}
		}
	}
	return out
}

func addPort(out map[string]string, port, where string) {
	if prev, ok := out[port]; ok {
		if !strings.Contains(prev, where) {
			out[port] = prev + ", " + where
		}
		return
	}
	out[port] = where
}

func yamlKey(trim string) (key, val string, ok bool) {
	if strings.HasPrefix(trim, "-") {
		return "", "", false
	}
	colon := strings.Index(trim, ":")
	if colon <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(trim[:colon])
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	return key, strings.TrimSpace(trim[colon+1:]), true
}

func hostPort(val string) (string, bool) {
	val = unquote(val)
	if val == "" {
		return "", false
	}
	_, port, err := net.SplitHostPort(val)
	if err != nil {
		return "", false
	}
	return plainPort(port)
}

func plainPort(val string) (string, bool) {
	val = unquote(val)
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 || n > 65535 {
		return "", false
	}
	return strconv.Itoa(n), true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func stripYAMLComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '#' {
			return strings.TrimRight(line[:i], " \t")
		}
	}
	return line
}
