package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestLoadDFSYaml(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../.."))
	files := []string{
		filepath.Join(root, "teamgramd/etc/dfs.yaml"),
		filepath.Join(root, "teamgramd/etc2/dfs.yaml"),
		filepath.Join(root, "app/service/dfs/etc/dfs.yaml"),
	}
	for _, name := range files {
		var c Config
		if err := conf.Load(name, &c, conf.UseEnv()); err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if c.Minio.Endpoint == "" || c.Minio.AccessKeyID == "" {
			t.Fatalf("%s: Minio endpoint or access key was not loaded: %#v", name, c.Minio)
		}
		if c.Minio.Bucket.Attachments == "" || c.Minio.Bucket.PublicMedia == "" {
			t.Fatalf("%s: bucket names were not loaded: %#v", name, c.Minio.Bucket)
		}
	}
}
