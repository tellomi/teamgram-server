package mtproto

import (
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestWalletCardRoundTrip(t *testing.T) {
	if got := uint32(GetClazzID(Predicate_wallet_setCardStickers, Layer)); got != crcWalletSetCardStickers {
		t.Fatalf("setCardStickers ctor 0x%x", got)
	}
	if got := uint32(GetClazzID(Predicate_wallet_card, Layer)); got != crcWalletCard {
		t.Fatalf("card ctor 0x%x", got)
	}

	coords := MakeTLMediaAreaCoordinates(&MediaAreaCoordinates{
		X: 0.1, Y: 0.2, W: 0.3, H: 0.4, Rotation: 15,
	}).To_MediaAreaCoordinates()
	req := &TLWalletSetCardStickers{
		Stickers: []*InputWalletCardSticker{{
			Coordinates: coords,
			NftAddress:  "EQ-nft",
		}},
		BgNftAddress: &wrapperspb.StringValue{Value: "EQ-bg"},
	}
	raw := encodeAt(t, req, Layer)
	if ctorOf(raw) != crcWalletSetCardStickers {
		t.Fatalf("request ctor 0x%x", ctorOf(raw))
	}
	gotReq, ok := NewDecodeBuf(raw).Object().(*TLWalletSetCardStickers)
	if !ok || len(gotReq.GetStickers()) != 1 || gotReq.GetStickers()[0].GetNftAddress() != "EQ-nft" {
		t.Fatalf("request %#v", gotReq)
	}
	if gotReq.GetBgNftAddress().GetValue() != "EQ-bg" {
		t.Fatalf("bg %q", gotReq.GetBgNftAddress())
	}
	gotCoords := gotReq.GetStickers()[0].GetCoordinates()
	if gotCoords.GetX() != 0.1 || gotCoords.GetW() != 0.3 || gotCoords.GetRotation() != 15 {
		t.Fatalf("coords %#v", gotCoords)
	}

	nft := MakeTLWalletNftItem(&Wallet_NftItem{
		Address: "EQ-nft", OwnerAddress: "owner", Index: "0",
	}).To_Wallet_NftItem()
	bg := MakeTLWalletNftItem(&Wallet_NftItem{
		Address: "EQ-bg", OwnerAddress: "owner", Index: "1",
	}).To_Wallet_NftItem()
	card := MakeWalletCard("", 0, []*Wallet_CardSticker{MakeWalletCardSticker(coords, nft)}, bg)
	raw = encodeAt(t, card, Layer)
	if ctorOf(raw) != crcWalletCard {
		t.Fatalf("card ctor 0x%x", ctorOf(raw))
	}
	gotCard, ok := NewDecodeBuf(raw).Object().(*Wallet_Card)
	if !ok || !gotCard.GetHasStickers() || !gotCard.GetHasBg() || gotCard.GetBalance() != 0 {
		t.Fatalf("card %#v", gotCard)
	}
	if gotCard.GetAddress() != "" || len(gotCard.GetStickers()) != 1 {
		t.Fatalf("card body %#v", gotCard)
	}
	if gotCard.GetStickers()[0].GetNft().GetAddress() != "EQ-nft" || gotCard.GetBgNftItem().GetAddress() != "EQ-bg" {
		t.Fatalf("nfts sticker=%s bg=%s", gotCard.GetStickers()[0].GetNft().GetAddress(), gotCard.GetBgNftItem().GetAddress())
	}
}
