package gnet

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPermAuthKeyIdUsesBoundKey(t *testing.T) {
	legacy := mtproto.NewAuthKeyInfo(469193, []byte{1, 2, 3, 4}, mtproto.AuthKeyTypePerm)
	legacy.PermAuthKeyId = 0
	if got := newAuthKeyUtil(legacy).PermAuthKeyId(); got != 469193 {
		t.Fatalf("legacy perm key %d", got)
	}

	temp := mtproto.NewAuthKeyInfo(88, []byte{5, 6, 7, 8}, mtproto.AuthKeyTypeTemp)
	if got := newAuthKeyUtil(temp).PermAuthKeyId(); got != 0 {
		t.Fatalf("unbound temp key %d", got)
	}
	temp.PermAuthKeyId = 469193
	if got := newAuthKeyUtil(temp).PermAuthKeyId(); got != 469193 {
		t.Fatalf("bound temp key %d", got)
	}
}
