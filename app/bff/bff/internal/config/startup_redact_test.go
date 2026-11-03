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
	configload.MustLoad(etc2(t, "bff.yaml"), &c)
	if !strings.Contains(c.ChannelMySQL, "teamgram:teamgram@tcp(") {
		t.Fatalf("fixture DSN: %s", c.ChannelMySQL)
	}
	logged := redact.Value(c).(Config)
	if !strings.Contains(c.ChannelMySQL, "teamgram:teamgram@tcp(") {
		t.Fatal("redact.Value mutated the live DSN")
	}
	raw := mustJSON(t, logged)
	if strings.Contains(raw, "teamgram:teamgram@") || strings.Contains(raw, "tellomi-turn-secret") {
		t.Fatalf("startup log leaked a secret: %s", raw)
	}
	if !strings.Contains(raw, "teamgram:***@tcp(mysql:3306)/teamgram") || !strings.Contains(raw, "bff.bff") {
		t.Fatalf("startup log dropped non-secrets: %s", raw)
	}
	if logged.Phone.RTMPURL != c.Phone.RTMPURL {
		t.Fatalf("RTMPURL %q", logged.Phone.RTMPURL)
	}
	if c.Phone.TurnSecret != "" && logged.Phone.TurnSecret != "***" {
		t.Fatalf("TurnSecret %q", logged.Phone.TurnSecret)
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

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
