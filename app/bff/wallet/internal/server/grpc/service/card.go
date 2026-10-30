package service

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/ledger"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (s *Service) WalletSetCardStickers(ctx context.Context, in *mtproto.TLWalletSetCardStickers) (*mtproto.Wallet_Card, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	if len(in.GetStickers()) > 32 {
		s.svcCtx.Ledger.Touch(id)
		return nil, rpcErr("STICKERS_TOO_MUCH")
	}
	card := ledger.Card{Stickers: make([]ledger.Sticker, 0, len(in.GetStickers()))}
	for _, row := range in.GetStickers() {
		st, err := stickerFrom(row)
		if err != nil {
			s.svcCtx.Ledger.Touch(id)
			return nil, err
		}
		card.Stickers = append(card.Stickers, st)
	}
	if in.GetBgNftAddress() != nil {
		bg := strings.TrimSpace(in.GetBgNftAddress().GetValue())
		if bg == "" || len(bg) > 128 {
			s.svcCtx.Ledger.Touch(id)
			return nil, rpcErr("BG_NFT_INVALID")
		}
		card.HasBg = true
		card.BgAddress = bg
	}
	if code := s.svcCtx.Ledger.SetCard(id, card); code != "" {
		return nil, fail(code)
	}
	return cardTL(card), nil
}

func (s *Service) ToncenterPerformApiRequest(ctx context.Context, in *mtproto.TLToncenterPerformApiRequest) (*mtproto.Toncenter_ApiResponse, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	if strings.TrimSpace(in.GetEndpoint()) == "" {
		return nil, rpcErr("ENDPOINT_EMPTY")
	}
	return nil, rpcErr("TONCENTER_UNAVAILABLE")
}

func (s *Service) ToncenterGetStreamingUrl(ctx context.Context, _ *mtproto.TLToncenterGetStreamingUrl) (*mtproto.Toncenter_StreamingUrl, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	s.svcCtx.Ledger.Touch(id)
	return nil, rpcErr("TONCENTER_UNAVAILABLE")
}

func stickerFrom(in *mtproto.InputWalletCardSticker) (ledger.Sticker, error) {
	if in == nil || in.GetCoordinates() == nil {
		return ledger.Sticker{}, rpcErr("STICKER_INVALID")
	}
	addr := strings.TrimSpace(in.GetNftAddress())
	if addr == "" || len(addr) > 128 {
		return ledger.Sticker{}, rpcErr("STICKER_INVALID")
	}
	c := in.GetCoordinates()
	if c.GetW() <= 0 || c.GetH() <= 0 {
		return ledger.Sticker{}, rpcErr("STICKER_INVALID")
	}
	st := ledger.Sticker{
		X: c.GetX(), Y: c.GetY(), W: c.GetW(), H: c.GetH(), Rotation: c.GetRotation(),
		NftAddress: addr,
	}
	if c.GetRadius() != nil {
		st.HasRadius = true
		st.Radius = c.GetRadius().GetValue()
	}
	return st, nil
}

func cardTL(card ledger.Card) *mtproto.Wallet_Card {
	rows := make([]*mtproto.Wallet_CardSticker, 0, len(card.Stickers))
	for _, st := range card.Stickers {
		coords := mtproto.MakeTLMediaAreaCoordinates(&mtproto.MediaAreaCoordinates{
			X: st.X, Y: st.Y, W: st.W, H: st.H, Rotation: st.Rotation,
		}).To_MediaAreaCoordinates()
		if st.HasRadius {
			coords.Radius = wrapperspb.Double(st.Radius)
		}
		nft := mtproto.MakeTLWalletNftItem(&mtproto.Wallet_NftItem{
			Address:      st.NftAddress,
			OwnerAddress: "",
			Index:        "0",
		}).To_Wallet_NftItem()
		rows = append(rows, mtproto.MakeWalletCardSticker(coords, nft))
	}
	var bg *mtproto.Wallet_NftItem
	if card.HasBg {
		bg = mtproto.MakeTLWalletNftItem(&mtproto.Wallet_NftItem{
			Address:      card.BgAddress,
			OwnerAddress: "",
			Index:        "0",
		}).To_Wallet_NftItem()
	}
	return mtproto.MakeWalletCard("", 0, rows, bg)
}
