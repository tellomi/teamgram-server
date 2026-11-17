package mtproto

import "testing"

func TestFromMessageResolvesTheReferencedID(t *testing.T) {
	user := MakeTLInputUserFromMessage(&InputUser{
		UserId: 2,
		MsgId:  4,
		Peer:   MakeTLInputPeerUser(&InputPeer{UserId: 8, AccessHash: 1}).To_InputPeer(),
	}).To_InputUser()
	if got := FromInputUser(1, user); got.PeerType != PEER_EMPTY || got.PeerId != 0 {
		t.Fatalf("historical from-message %#v", got)
	}
	ResolveFromMessage = func() bool { return true }
	t.Cleanup(func() { ResolveFromMessage = nil })

	got := FromInputUser(1, user)
	if got.PeerType != PEER_USER || got.PeerId != 2 {
		t.Fatalf("inputUserFromMessage %#v", got)
	}

	self := FromInputUser(1, MakeTLInputUserSelf(nil).To_InputUser())
	if self.PeerType != PEER_SELF || self.PeerId != 1 {
		t.Fatalf("self %#v", self)
	}

	peer := MakeTLInputPeerUserFromMessage(&InputPeer{
		UserId: 2,
		MsgId:  4,
		Peer:   MakeTLInputPeerChat(&InputPeer{ChatId: 3}).To_InputPeer(),
	}).To_InputPeer()
	p := FromInputPeer2(1, peer)
	if p.PeerType != PEER_USER || p.PeerId != 2 {
		t.Fatalf("inputPeerUserFromMessage %#v", p)
	}

	ch := MakeTLInputPeerChannelFromMessage(&InputPeer{
		ChannelId: 5,
		MsgId:     6,
		Peer:      MakeTLInputPeerChat(&InputPeer{ChatId: 3}).To_InputPeer(),
	}).To_InputPeer()
	p = FromInputPeer2(1, ch)
	if p.PeerType != PEER_CHANNEL || p.PeerId != 5 {
		t.Fatalf("inputPeerChannelFromMessage %#v", p)
	}
}
