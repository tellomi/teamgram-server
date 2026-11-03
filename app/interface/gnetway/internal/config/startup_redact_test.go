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
	configload.MustLoad(etc2(t, "gnetway.yaml"), &c)
	if len(c.RSAKey) == 0 || c.RSAKey[0].KeyFile == "" {
		t.Fatal("fixture has no RSA key file")
	}
	logged := redact.Value(c).(Config)
	if c.RSAKey[0].KeyFile == "" {
		t.Fatal("redact.Value cleared the live key path")
	}
	if logged.RSAKey[0].KeyFile != "***" || logged.RSAKey[0].KeyFingerprint != "***" {
		t.Fatalf("rsa key: %+v", logged.RSAKey[0])
	}
	if logged.Gnetway == nil || len(logged.Gnetway.Server) == 0 {
		t.Fatal("gnetway listeners dropped")
	}
	raw, err := json.Marshal(logged)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, c.RSAKey[0].KeyFile) || strings.Contains(text, "12240908862933197005") {
		t.Fatalf("startup log leaked the key path or fingerprint: %s", text)
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
