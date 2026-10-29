package routecheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSessionRoutesHaveBFFServices(t *testing.T) {
	root := repoRoot(t)
	registered := registeredBFFServices(t, filepath.Join(root, "app/bff/bff/internal/server/server.go"))
	yamls := []string{
		"teamgramd/etc/session.yaml",
		"teamgramd/etc2/session.yaml",
		"app/interface/session/etc/session.yaml",
	}
	var sawEmojiCategories bool
	for _, rel := range yamls {
		routes := bffRoutes(t, filepath.Join(root, rel))
		if len(routes) == 0 {
			t.Fatalf("%s has no bff routes", rel)
		}
		for _, route := range routes {
			name := strings.TrimPrefix(route, "/mtproto.")
			if _, ok := registered[name]; !ok {
				t.Errorf("%s routes %s to bff, but bff does not register %s", rel, route, name)
			}
			if name == "RPCEmojiCategories" {
				sawEmojiCategories = true
			}
		}
	}
	if !sawEmojiCategories {
		t.Fatal("session.yaml does not route RPCEmojiCategories")
	}
	if _, ok := registered["RPCEmojiCategories"]; !ok {
		t.Fatal("bff does not register RPCEmojiCategories")
	}
}

func registeredBFFServices(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`RegisterRPC([A-Za-z0-9]+)Server`).FindAllStringSubmatch(string(body), -1)
	out := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		out["RPC"+m[1]] = struct{}{}
	}
	if len(out) == 0 {
		t.Fatalf("no RegisterRPC calls in %s", path)
	}
	return out
}

func bffRoutes(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	for _, line := range strings.Split(string(body), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if !strings.Contains(trim, `"bff.bff"`) {
			continue
		}
		start := strings.Index(trim, `"`)
		end := strings.Index(trim[start+1:], `"`)
		if start < 0 || end < 0 {
			continue
		}
		route := trim[start+1 : start+1+end]
		if strings.HasPrefix(route, "/mtproto.") {
			routes = append(routes, route)
		}
	}
	return routes
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "teamgramd/etc/session.yaml")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
