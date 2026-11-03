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
	configload.MustLoad(etc2(t, "biz.yaml"), &c)
	logged := redact.Value(c).(Config)
	if !strings.Contains(c.Mysql.DSN, "teamgram:teamgram@tcp(") {
		t.Fatal("redact.Value mutated the live DSN")
	}
	raw, err := json.Marshal(logged)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "teamgram:teamgram@") {
		t.Fatalf("startup log leaked the DSN password: %s", text)
	}
	if !strings.Contains(text, "teamgram:***@tcp(mysql:3306)/teamgram") {
		t.Fatalf("redacted DSN missing: %s", text)
	}
	if len(c.KV) > 0 && (len(logged.KV) == 0 || logged.KV[0].Host != c.KV[0].Host) {
		t.Fatalf("redis host changed: %+v", logged.KV)
	}
}

func etc2(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..", "teamgramd", "etc2", name)
}
