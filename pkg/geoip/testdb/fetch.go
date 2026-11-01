// Package testdb loads the pinned ip2region xdb for tests. The files are
// downloaded by scripts/fetch-ip2region-xdb.sh, which checks the SHA256 of
// the official v3.18.0 release. Production binaries do not import this package.
package testdb

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/geoip"
)

var (
	openOnce sync.Once
	shared   *geoip.Resolver
)

// Open returns an in-memory ip2region resolver backed by the pinned xdb.
// Repeated calls in one test process share the loaded database.
func Open(t *testing.T) *geoip.Resolver {
	t.Helper()
	v4, v6 := Files(t)
	openOnce.Do(func() {
		shared = geoip.Open(geoip.Options{
			V4Path:   v4,
			V6Path:   v6,
			MMDBPath: filepath.Join(os.TempDir(), "tellomi-geoip-absent.mmdb"),
		})
	})
	if shared == nil {
		t.Fatal("ip2region resolver was not loaded")
	}
	return shared
}

// Files returns the v4 and v6 xdb paths, fetching them when the checksum
// in the tree does not already match.
func Files(t *testing.T) (v4, v6 string) {
	t.Helper()
	root := moduleRoot(t)
	dest := filepath.Join(root, "data", "ip2region")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "fetch-ip2region-xdb.sh"), dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fetch ip2region xdb: %v\n%s", err, out)
	}
	return filepath.Join(dest, "ip2region_v4.xdb"), filepath.Join(dest, "ip2region_v6.xdb")
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "scripts", "fetch-ip2region-xdb.sh")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}
}
