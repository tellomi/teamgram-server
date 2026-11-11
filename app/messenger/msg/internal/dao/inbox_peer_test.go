package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestInboxMessagePeerPrivateChatIsTheSender(t *testing.T) {
	fromID, toID := int64(717), int64(727)
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: toID}
	got := inboxMessagePeer(fromID, peer)
	if got == nil || got.PeerType != mtproto.PEER_USER || got.PeerId != fromID {
		t.Fatalf("private inbox peer = %#v, want sender %d", got, fromID)
	}
	if peer.PeerId != toID {
		t.Fatal("inboxMessagePeer must not rewrite the input peer")
	}
}

func TestInboxMessagePeerGroupKeepsChatID(t *testing.T) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 42}
	got := inboxMessagePeer(717, peer)
	if got != peer || got.PeerId != 42 || got.PeerType != mtproto.PEER_CHAT {
		t.Fatalf("group inbox peer = %#v", got)
	}
}

func TestInboxMessagePeerNil(t *testing.T) {
	if inboxMessagePeer(717, nil) != nil {
		t.Fatal("nil peer should stay nil")
	}
}
