package saved

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestParentSpecified(t *testing.T) {
	if ParentSpecified(nil) || ParentSpecified(&mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerEmpty}) {
		t.Fatal("empty parent is Saved Messages")
	}
	if !ParentSpecified(&mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChannel, ChannelId: 3}) {
		t.Fatal("channel parent is a monoforum")
	}
}

func TestSavedPeer(t *testing.T) {
	if SavedPeer(nil) || SavedPeer(&mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY}) {
		t.Fatal("empty peer")
	}
	if !SavedPeer(&mtproto.PeerUtil{PeerType: mtproto.PEER_SELF}) {
		t.Fatal("self")
	}
	if SavedPeer(&mtproto.PeerUtil{PeerType: mtproto.PEER_USER}) {
		t.Fatal("user 0")
	}
	if !SavedPeer(&mtproto.PeerUtil{PeerType: mtproto.PEER_CHANNEL, PeerId: 9}) {
		t.Fatal("channel")
	}
}

func TestFullList(t *testing.T) {
	if !FullList(0, 20) || !FullList(19, 20) {
		t.Fatal("short page is the full list")
	}
	if FullList(20, 20) {
		t.Fatal("full page is a slice")
	}
}

func TestHashStableAndZero(t *testing.T) {
	rows := []Row{
		{Pinned: true, PeerType: mtproto.PEER_USER, PeerID: 7, Top: 10},
		{PeerType: mtproto.PEER_CHAT, PeerID: 4, Top: 3},
	}
	h := Hash(rows)
	if h == 0 || h != Hash(rows) {
		t.Fatalf("hash %d", h)
	}
	if NotModified(0, rows) {
		t.Fatal("hash 0 must return the list")
	}
	if !NotModified(h, rows) {
		t.Fatal("matching hash")
	}
	rows[1].Top = 4
	if NotModified(h, rows) {
		t.Fatal("top_message is part of the hash")
	}
}

func TestDeleteWindow(t *testing.T) {
	if InDeleteWindow(5, 100, 0, 0, 0) {
		t.Fatal("max_id 0 deletes nothing")
	}
	if !InDeleteWindow(5, 100, 5, 0, 0) || InDeleteWindow(6, 100, 5, 0, 0) {
		t.Fatal("id bound")
	}
	if InDeleteWindow(5, 50, 9, 80, 0) {
		t.Fatal("older than min_date")
	}
	if InDeleteWindow(5, 200, 9, 0, 150) {
		t.Fatal("newer than max_date")
	}
	if !InDeleteWindow(5, 100, 9, 80, 150) {
		t.Fatal("inside the window")
	}
}

func TestNormalizeLimit(t *testing.T) {
	if NormalizeLimit(0, PageCap) != PageCap || NormalizeLimit(500, PageCap) != PageCap {
		t.Fatal("cap")
	}
	if NormalizeLimit(20, PageCap) != 20 {
		t.Fatal("keep")
	}
}
