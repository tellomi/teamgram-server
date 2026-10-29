package relay

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"testing"
	"time"
)

func TestTURNPasswordMatchesCoturn(t *testing.T) {
	user := TURNUsername("42", time.Hour, time.Unix(1_700_000_000, 0))
	if user != "1700003600:42" {
		t.Fatal(user)
	}
	got := TURNPassword("tellomi-turn-secret", user)
	mac := hmac.New(sha1.New, []byte("tellomi-turn-secret"))
	_, _ = mac.Write([]byte(user))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want || got == "" {
		t.Fatalf("got %s want %s", got, want)
	}
}
