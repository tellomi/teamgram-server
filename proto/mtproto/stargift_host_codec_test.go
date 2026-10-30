package mtproto

import "testing"

func TestStarGiftHostRoundTrip(t *testing.T) {
	gift := MakeTLInputSavedStarGiftUser(&InputSavedStarGift{MsgId: 8}).To_InputSavedStarGift()
	peer := MakeTLInputPeerSelf(nil).To_InputPeer()

	del := &TLPaymentsDeleteStarGift{Stargift: gift}
	raw := encodeAt(t, del, Layer)
	if ctorOf(raw) != crcPaymentsDeleteStarGift {
		t.Fatalf("delete ctor 0x%x", ctorOf(raw))
	}
	gotDel, ok := NewDecodeBuf(raw).Object().(*TLPaymentsDeleteStarGift)
	if !ok || gotDel.GetStargift().GetMsgId() != 8 {
		t.Fatalf("delete %#v", gotDel)
	}

	withdraw := &TLPaymentsWithdrawStarGift{KeepOnProfile: true, Stargift: gift, TmpPassword: []byte{1, 2, 3}}
	raw = encodeAt(t, withdraw, Layer)
	gotW, ok := NewDecodeBuf(raw).Object().(*TLPaymentsWithdrawStarGift)
	if !ok || !gotW.GetKeepOnProfile() || len(gotW.GetTmpPassword()) != 3 || gotW.GetStargift().GetMsgId() != 8 {
		t.Fatalf("withdraw %#v", gotW)
	}

	deposit := &TLPaymentsDepositStarGift{HideSender: true, Peer: peer, Slug: "Plush-1"}
	raw = encodeAt(t, deposit, Layer)
	if ctorOf(raw) != crcPaymentsDepositStarGift {
		t.Fatalf("deposit ctor 0x%x", ctorOf(raw))
	}
	gotD, ok := NewDecodeBuf(raw).Object().(*TLPaymentsDepositStarGift)
	if !ok || !gotD.GetHideSender() || gotD.GetSlug() != "Plush-1" || gotD.GetPeer().GetPredicateName() != Predicate_inputPeerSelf {
		t.Fatalf("deposit %#v", gotD)
	}

	display := &TLPaymentsDisplayStarGift{HasPeer: true, Peer: peer, Slug: "Plush-1"}
	raw = encodeAt(t, display, Layer)
	gotS, ok := NewDecodeBuf(raw).Object().(*TLPaymentsDisplayStarGift)
	if !ok || !gotS.GetHasPeer() || gotS.GetSlug() != "Plush-1" {
		t.Fatalf("display %#v", gotS)
	}

	bare := &TLPaymentsDisplayStarGift{Slug: "Plush-1"}
	raw = encodeAt(t, bare, Layer)
	gotB, ok := NewDecodeBuf(raw).Object().(*TLPaymentsDisplayStarGift)
	if !ok || gotB.GetHasPeer() || gotB.GetPeer() != nil || gotB.GetSlug() != "Plush-1" {
		t.Fatalf("display without peer %#v", gotB)
	}
}
