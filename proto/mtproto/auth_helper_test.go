package mtproto

import (
	"strings"
	"testing"
)

func TestBoundPermAuthKeyId(t *testing.T) {
	perm := NewAuthKeyInfo(469193, []byte{1}, AuthKeyTypePerm)
	if got := perm.BoundPermAuthKeyId(); got != 469193 {
		t.Fatalf("perm key resolves to %d", got)
	}

	// Legacy perm row saved before PermAuthKeyId was filled in.
	legacy := NewAuthKeyInfo(469193, []byte{1}, AuthKeyTypePerm)
	legacy.PermAuthKeyId = 0
	if got := legacy.BoundPermAuthKeyId(); got != 469193 {
		t.Fatalf("legacy perm key resolves to %d", got)
	}

	temp := NewAuthKeyInfo(8412457259831554854, []byte{2}, AuthKeyTypeTemp)
	if got := temp.BoundPermAuthKeyId(); got != 0 {
		t.Fatalf("unbound temp key resolves to %d", got)
	}
	temp.PermAuthKeyId = 469193
	if got := temp.BoundPermAuthKeyId(); got != 469193 {
		t.Fatalf("bound temp key resolves to %d, want perm 469193", got)
	}

	media := NewAuthKeyInfo(99, []byte{3}, AuthKeyTypeMediaTemp)
	media.PermAuthKeyId = 469193
	if got := media.BoundPermAuthKeyId(); got != 469193 {
		t.Fatalf("media temp key resolves to %d", got)
	}
}

func TestAuthKeyLogLabelOmitsKeyBytes(t *testing.T) {
	secret := []byte("sentinel-key-bytes-not-for-logs")
	info := NewAuthKeyInfo(42, secret, AuthKeyTypePerm)
	info.TempAuthKeyId = 7
	label := AuthKeyLogLabel(info)
	if strings.Contains(label, string(secret)) {
		t.Fatal("log label contains key bytes")
	}
	if !strings.Contains(label, "auth_key_id=42") || !strings.Contains(label, "perm_auth_key_id=42") {
		t.Fatalf("log label missing key id: %s", label)
	}
	if AuthKeyLogLabel(nil) != "auth_key_id=0" {
		t.Fatal("nil key label")
	}
}
