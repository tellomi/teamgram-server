package config

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/configload"
	"github.com/teamgram/teamgram-server/pkg/redact"
)

func TestStartupLogRedactsSecrets(t *testing.T) {
	var c Config
	configload.MustLoad(etc2(t, "dfs.yaml"), &c)
	secret := c.Minio.SecretAccessKey
	if secret == "" {
		t.Fatal("fixture has no minio secret")
	}
	logged := redact.Value(c).(Config)
	if c.Minio.SecretAccessKey != secret {
		t.Fatal("redact.Value mutated the live minio secret")
	}
	if logged.Minio.SecretAccessKey != "***" || logged.Minio.AccessKeyID != "***" {
		t.Fatalf("minio keys access %q secret %q", logged.Minio.AccessKeyID, logged.Minio.SecretAccessKey)
	}
	if logged.Minio.Endpoint != c.Minio.Endpoint {
		t.Fatalf("endpoint %q", logged.Minio.Endpoint)
	}
	raw, err := json.Marshal(logged)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("startup log leaked the minio secret: %s", raw)
	}
}

func etc2(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "teamgramd", "etc2", name)
}
