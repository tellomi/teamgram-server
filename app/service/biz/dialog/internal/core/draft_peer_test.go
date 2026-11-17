package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSavedDraftPeer(t *testing.T) {
	typ, id := savedDraftPeer(7, mtproto.PEER_SELF, 7)
	if typ != mtproto.PEER_USER || id != 7 {
		t.Fatalf("self: %d %d", typ, id)
	}
	typ, id = savedDraftPeer(7, mtproto.PEER_USER, 0)
	if typ != mtproto.PEER_USER || id != 7 {
		t.Fatalf("empty user: %d %d", typ, id)
	}
	typ, id = savedDraftPeer(7, mtproto.PEER_USER, 9)
	if typ != mtproto.PEER_USER || id != 9 {
		t.Fatalf("other: %d %d", typ, id)
	}
	typ, id = savedDraftPeer(7, mtproto.PEER_CHAT, 4)
	if typ != mtproto.PEER_CHAT || id != 4 {
		t.Fatalf("chat: %d %d", typ, id)
	}
}
