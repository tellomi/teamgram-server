package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDraftStorePeer(t *testing.T) {
	peer := draftStorePeer(7, &mtproto.PeerUtil{PeerType: mtproto.PEER_SELF, PeerId: 7})
	if peer.PeerType != mtproto.PEER_USER || peer.PeerId != 7 {
		t.Fatalf("%#v", peer)
	}
	other := draftStorePeer(7, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 3})
	if other.PeerType != mtproto.PEER_CHAT || other.PeerId != 3 {
		t.Fatalf("%#v", other)
	}
}
