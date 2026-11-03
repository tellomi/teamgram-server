package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestNormalizePermAuthKeyFillsLegacyPermanentRow(t *testing.T) {
	legacy := mtproto.NewAuthKeyInfo(469193, []byte{1}, mtproto.AuthKeyTypePerm)
	legacy.PermAuthKeyId = 0
	normalizePermAuthKey(legacy)
	if legacy.PermAuthKeyId != 469193 {
		t.Fatalf("perm id %d", legacy.PermAuthKeyId)
	}

	temp := mtproto.NewAuthKeyInfo(99, []byte{2}, mtproto.AuthKeyTypeTemp)
	normalizePermAuthKey(temp)
	if temp.PermAuthKeyId != 0 {
		t.Fatalf("unbound temp key became %d", temp.PermAuthKeyId)
	}
	temp.PermAuthKeyId = 469193
	normalizePermAuthKey(temp)
	if temp.PermAuthKeyId != 469193 {
		t.Fatalf("bound temp key changed to %d", temp.PermAuthKeyId)
	}
}
