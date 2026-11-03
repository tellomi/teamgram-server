package discoverauth

import (
	"encoding/base64"
	"testing"
)

func TestLoadKeys(t *testing.T) {
	sign := base64.StdEncoding.EncodeToString(seq(1, 32))
	pseudo := base64.StdEncoding.EncodeToString(seq(2, 32))
	t.Setenv(EnvSigningKey, sign)
	t.Setenv(EnvPseudonymKey, pseudo)
	keys, err := LoadKeys()
	if err != nil || len(keys.Signing) != 32 || len(keys.Pseudonym) != 32 {
		t.Fatal(err)
	}
	t.Setenv(EnvPseudonymKey, sign)
	if _, err = LoadKeys(); err == nil {
		t.Fatal("equal keys accepted")
	}
	t.Setenv(EnvPseudonymKey, base64.StdEncoding.EncodeToString(seq(2, 16)))
	if _, err = LoadKeys(); err == nil {
		t.Fatal("short key accepted")
	}
	t.Setenv(EnvSigningKey, "")
	t.Setenv(EnvPseudonymKey, pseudo)
	if _, err = LoadKeys(); err == nil {
		t.Fatal("empty key accepted")
	}
	if Enabled() {
		t.Fatal("flag defaulted on")
	}
	t.Setenv(EnvEnabled, "1")
	if !Enabled() {
		t.Fatal("flag=1 did not enable")
	}
	t.Setenv(EnvEnabled, "true")
	if Enabled() {
		t.Fatal("flag true enabled")
	}
}
