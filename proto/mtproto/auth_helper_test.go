package mtproto

import "testing"

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
