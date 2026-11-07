package usernamepolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Chat message handlers must not consult this package. Titles, bios, and
// usernames are checked at the profile and username handlers. D-2 / D-26:
// message text is not filtered, and nothing here rewrites stored names.
func TestChatPackagesDoNotImportUsernamePolicy(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "app", "bff", "messages"),
		filepath.Join("..", "..", "app", "messenger"),
	}
	var hits []string
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), "usernamepolicy") {
				hits = append(hits, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(hits) != 0 {
		t.Fatalf("chat code references usernamepolicy: %s", strings.Join(hits, ", "))
	}
}
