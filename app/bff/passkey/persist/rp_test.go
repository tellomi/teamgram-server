package persist

import (
	"strings"
	"testing"
)

func TestAndroidOriginRequiresConfiguredHash(t *testing.T) {
	t.Setenv("TELLOMI_PASSKEY_ANDROID_HASHES", "")
	if originAllowed("android:apk-key-hash:client-supplied") {
		t.Fatal("an unconfigured android hash was accepted")
	}
	wa, err := newWebAuthn("android:apk-key-hash:client-supplied")
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range wa.Config.RPOrigins {
		if strings.HasPrefix(origin, "android:") {
			t.Fatalf("client origin was appended: %v", wa.Config.RPOrigins)
		}
	}
	t.Setenv("TELLOMI_PASSKEY_ANDROID_HASHES", "configured-hash")
	if !originAllowed("android:apk-key-hash:configured-hash") {
		t.Fatal("configured hash was rejected")
	}
	if originAllowed("android:apk-key-hash:other") {
		t.Fatal("a different hash was accepted")
	}
	wa, err = newWebAuthn("android:apk-key-hash:configured-hash")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, origin := range wa.Config.RPOrigins {
		if origin == "android:apk-key-hash:configured-hash" {
			found = true
		}
	}
	if !found {
		t.Fatalf("configured origin missing: %v", wa.Config.RPOrigins)
	}
}
