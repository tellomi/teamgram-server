package strippedbackfill

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackfillLayoutMatchesContainer(t *testing.T) {
	if ContainerDFSConfig != "/app/etc2/dfs.yaml" {
		t.Fatal(ContainerDFSConfig)
	}
	if ContainerRedis != "redis:6379" {
		t.Fatal(ContainerRedis)
	}
	root := repoRoot(t)
	runall := read(t, filepath.Join(root, "teamgramd/bin/runall-docker.sh"))
	if !strings.Contains(runall, "./dfs -f=../etc2/dfs.yaml") {
		t.Fatal("dfs in the container is not started from etc2")
	}
	etc2 := read(t, filepath.Join(root, "teamgramd/etc2/dfs.yaml"))
	if !strings.Contains(etc2, "Host: "+ContainerRedis) || !strings.Contains(etc2, "tcp(mysql:3306)") {
		t.Fatal("etc2/dfs.yaml does not use the compose redis and mysql hosts")
	}
	host := read(t, filepath.Join(root, "teamgramd/etc/dfs.yaml"))
	if !strings.Contains(host, "127.0.0.1:6379") || !strings.Contains(host, "127.0.0.1:3306") {
		t.Fatal("host-network dfs.yaml no longer points at loopback; re-check the backfill command")
	}
	compose := read(t, filepath.Join(root, "docker-compose-env.yaml"))
	if !strings.Contains(compose, "\n  redis:\n") || !strings.Contains(compose, "\"6379:6379\"") {
		t.Fatal("compose redis service is not redis on 6379")
	}
	mainSrc := read(t, filepath.Join(root, "app/service/dfs/cmd/backfillstripped/main.go"))
	if !strings.Contains(mainSrc, "strippedbackfill.ContainerDFSConfig") {
		t.Fatal("backfillstripped -f default is not the container yaml")
	}
	if strings.Contains(mainSrc, "/app/etc/dfs.yaml") || strings.Contains(mainSrc, "127.0.0.1:6379") {
		t.Fatal("backfillstripped still documents the host-network paths")
	}
	doc := read(t, filepath.Join(root, "docs/official-avatars.md"))
	if strings.Contains(doc, "/app/etc/dfs.yaml") || strings.Contains(doc, "127.0.0.1:6379") {
		t.Fatal("official-avatars.md still tells operators to use the host-network paths")
	}
	if !strings.Contains(doc, ContainerDFSConfig) || !strings.Contains(doc, ContainerRedis) {
		t.Fatal("official-avatars.md is missing the container dfs yaml or redis address")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
