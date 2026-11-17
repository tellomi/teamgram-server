package mtproto

import "testing"

func TestParseInputNotifyForumTopic(t *testing.T) {
	in := MakeTLInputNotifyForumTopic(&InputNotifyPeer{
		Peer:     MakeTLInputPeerChannel(&InputPeer{ChannelId: 9, AccessHash: 3}).To_InputPeer(),
		TopMsgId: 12,
	}).To_InputNotifyPeer()
	peer, top := ParseInputNotifyPeer(1, in)
	if peer.PeerType != PEER_CHANNEL || peer.PeerId != 9 || top != 12 {
		t.Fatalf("peer %+v top %d", peer, top)
	}
	got := NotifyPeerWithTopic(peer, top)
	if got.GetPredicateName() != Predicate_notifyForumTopic || got.GetTopMsgId() != 12 || got.GetPeer().GetChannelId() != 9 {
		t.Fatalf("notify %+v", got)
	}

	dialog, dialogTop := ParseInputNotifyPeer(1, MakeTLInputNotifyPeer(&InputNotifyPeer{
		Peer: MakeTLInputPeerChannel(&InputPeer{ChannelId: 9}).To_InputPeer(),
	}).To_InputNotifyPeer())
	if dialog.PeerType != PEER_CHANNEL || dialog.PeerId != 9 || dialogTop != 0 {
		t.Fatalf("dialog %+v %d", dialog, dialogTop)
	}
	plain := NotifyPeerWithTopic(dialog, 0)
	if plain.GetPredicateName() != Predicate_notifyPeer {
		t.Fatalf("dialog notify %s", plain.GetPredicateName())
	}

	_, zero := ParseInputNotifyPeer(1, MakeTLInputNotifyForumTopic(&InputNotifyPeer{
		Peer: MakeTLInputPeerChannel(&InputPeer{ChannelId: 9}).To_InputPeer(),
	}).To_InputNotifyPeer())
	if zero != 0 {
		t.Fatalf("missing topic id %d", zero)
	}
	unknown, _ := ParseInputNotifyPeer(1, MakeTLInputNotifyForumTopic(&InputNotifyPeer{TopMsgId: 4}).To_InputNotifyPeer())
	if unknown.PeerType != PEER_UNKNOWN {
		t.Fatalf("nil inner peer %+v", unknown)
	}
}
