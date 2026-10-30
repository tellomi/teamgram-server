package mtproto

import (
	"fmt"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Layer 232 wallet card constructors. See schema.tl.walletcard.proto.
//
//	inputWalletCardSticker#57df45a7 coordinates:MediaAreaCoordinates nft_address:string = InputWalletCardSticker;
//	wallet.cardSticker#f3010cba coordinates:MediaAreaCoordinates nft:wallet.NftItem = wallet.CardSticker;
//	wallet.card#8ed043b1 flags:# stickers:flags.0?Vector<wallet.CardSticker> bg_nft_item:flags.1?wallet.NftItem address:string balance:long = wallet.Card;
//	wallet.setCardStickers#68509ac flags:# stickers:Vector<InputWalletCardSticker> bg_nft_address:flags.0?string = wallet.Card;
const (
	Predicate_inputWalletCardSticker        = "inputWalletCardSticker"
	Predicate_wallet_cardSticker            = "wallet_cardSticker"
	Predicate_wallet_card                   = "wallet_card"
	Predicate_wallet_setCardStickers        = "wallet_setCardStickers"
	crcInputWalletCardSticker        uint32 = 0x57df45a7
	crcWalletCardSticker             uint32 = 0xf3010cba
	crcWalletCard                    uint32 = 0x8ed043b1
	crcWalletSetCardStickers         uint32 = 0x068509ac
)

func init() {
	registerLayer232Extra(Predicate_inputWalletCardSticker, crcInputWalletCardSticker, func(c int32) TLObject {
		return &InputWalletCardSticker{PredicateName: Predicate_inputWalletCardSticker, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_wallet_cardSticker, crcWalletCardSticker, func(c int32) TLObject {
		return &Wallet_CardSticker{PredicateName: Predicate_wallet_cardSticker, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_wallet_card, crcWalletCard, func(c int32) TLObject {
		return &Wallet_Card{PredicateName: Predicate_wallet_card, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_wallet_setCardStickers, crcWalletSetCardStickers, func(c int32) TLObject {
		return &TLWalletSetCardStickers{Constructor: TLConstructor(c)}
	})
}

// MakeWalletCardSticker builds wallet.cardSticker.
func MakeWalletCardSticker(coords *MediaAreaCoordinates, nft *Wallet_NftItem) *Wallet_CardSticker {
	return &Wallet_CardSticker{
		PredicateName: Predicate_wallet_cardSticker,
		Constructor:   ctorID(crcWalletCardSticker),
		Coordinates:   coords,
		Nft:           nft,
	}
}

// MakeWalletCard builds wallet.card. The stickers flag is set so an empty
// list is distinct from a card that never stored stickers.
func MakeWalletCard(address string, balance int64, stickers []*Wallet_CardSticker, bg *Wallet_NftItem) *Wallet_Card {
	if stickers == nil {
		stickers = []*Wallet_CardSticker{}
	}
	return &Wallet_Card{
		PredicateName: Predicate_wallet_card,
		Constructor:   ctorID(crcWalletCard),
		Stickers:      stickers,
		HasStickers:   true,
		BgNftItem:     bg,
		HasBg:         bg != nil,
		Address:       address,
		Balance:       balance,
	}
}

// Encode writes inputWalletCardSticker#57df45a7.
func (m *InputWalletCardSticker) Encode(x *EncodeBuf, layer int32) error {
	if m == nil || m.GetCoordinates() == nil {
		return fmt.Errorf("inputWalletCardSticker: missing coordinates")
	}
	x.UInt(crcInputWalletCardSticker)
	if err := m.GetCoordinates().Encode(x, layer); err != nil {
		return err
	}
	x.String(m.GetNftAddress())
	return nil
}

// Decode reads coordinates and nft_address. A nested value starts at constructor 0.
func (m *InputWalletCardSticker) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcInputWalletCardSticker {
		return fmt.Errorf("inputWalletCardSticker: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_inputWalletCardSticker
	coords := &MediaAreaCoordinates{}
	if err := coords.Decode(dBuf); err != nil {
		return err
	}
	m.Coordinates = coords
	m.NftAddress = dBuf.String()
	return dBuf.GetError()
}

// Encode writes wallet.cardSticker#f3010cba.
func (m *Wallet_CardSticker) Encode(x *EncodeBuf, layer int32) error {
	if m == nil || m.GetCoordinates() == nil || m.GetNft() == nil {
		return fmt.Errorf("wallet.cardSticker: missing coordinates or nft")
	}
	x.UInt(crcWalletCardSticker)
	if err := m.GetCoordinates().Encode(x, layer); err != nil {
		return err
	}
	return m.GetNft().Encode(x, layer)
}

// Decode reads coordinates and nft. A nested value starts at constructor 0.
func (m *Wallet_CardSticker) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcWalletCardSticker {
		return fmt.Errorf("wallet.cardSticker: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_wallet_cardSticker
	coords := &MediaAreaCoordinates{}
	if err := coords.Decode(dBuf); err != nil {
		return err
	}
	m.Coordinates = coords
	nft := &Wallet_NftItem{}
	if err := nft.Decode(dBuf); err != nil {
		return err
	}
	m.Nft = nft
	return dBuf.GetError()
}

// Encode writes wallet.card#8ed043b1.
func (m *Wallet_Card) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = MakeWalletCard("", 0, nil, nil)
	}
	x.UInt(crcWalletCard)
	var flags uint32
	if m.GetHasStickers() || len(m.GetStickers()) > 0 {
		flags |= 1 << 0
	}
	if m.GetHasBg() || m.GetBgNftItem() != nil {
		flags |= 1 << 1
	}
	x.UInt(flags)
	if flags&(1<<0) != 0 {
		rows := m.GetStickers()
		x.Int(int32(CRC32_vector))
		x.Int(int32(len(rows)))
		for _, row := range rows {
			if err := row.Encode(x, layer); err != nil {
				return err
			}
		}
	}
	if flags&(1<<1) != 0 {
		if m.GetBgNftItem() == nil {
			return fmt.Errorf("wallet.card: missing bg_nft_item")
		}
		if err := m.GetBgNftItem().Encode(x, layer); err != nil {
			return err
		}
	}
	x.String(m.GetAddress())
	x.Long(m.GetBalance())
	return nil
}

// Decode reads wallet.card. Object() has already consumed the constructor
// when Constructor is set.
func (m *Wallet_Card) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcWalletCard {
		return fmt.Errorf("wallet.card: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_wallet_card
	flags := dBuf.UInt()
	if flags&(1<<0) != 0 {
		m.HasStickers = true
		rows, err := decodeCardStickers(dBuf)
		if err != nil {
			return err
		}
		m.Stickers = rows
	}
	if flags&(1<<1) != 0 {
		m.HasBg = true
		nft := &Wallet_NftItem{}
		if err := nft.Decode(dBuf); err != nil {
			return err
		}
		m.BgNftItem = nft
	}
	m.Address = dBuf.String()
	m.Balance = dBuf.Long()
	return dBuf.GetError()
}

// Encode writes wallet.setCardStickers#68509ac.
func (m *TLWalletSetCardStickers) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLWalletSetCardStickers{}
	}
	x.UInt(crcWalletSetCardStickers)
	var flags uint32
	if m.GetBgNftAddress() != nil {
		flags |= 1 << 0
	}
	x.UInt(flags)
	rows := m.GetStickers()
	x.Int(int32(CRC32_vector))
	x.Int(int32(len(rows)))
	for _, row := range rows {
		if row == nil {
			return fmt.Errorf("wallet.setCardStickers: nil sticker")
		}
		if err := row.Encode(x, layer); err != nil {
			return err
		}
	}
	if flags&(1<<0) != 0 {
		x.String(m.GetBgNftAddress().GetValue())
	}
	return nil
}

// Decode reads flags, stickers, and the optional background address.
// Object() has already consumed the constructor.
func (m *TLWalletSetCardStickers) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	rows, err := decodeInputCardStickers(dBuf)
	if err != nil {
		return err
	}
	m.Stickers = rows
	if flags&(1<<0) != 0 {
		m.BgNftAddress = &wrapperspb.StringValue{Value: dBuf.String()}
	}
	return dBuf.GetError()
}

func decodeInputCardStickers(dBuf *DecodeBuf) ([]*InputWalletCardSticker, error) {
	c := dBuf.Int()
	if c != int32(CRC32_vector) {
		return nil, fmt.Errorf("inputWalletCardSticker: expected vector, got 0x%x", uint32(c))
	}
	n := dBuf.Int()
	if n < 0 || n > maxVectorLen {
		return nil, fmt.Errorf("inputWalletCardSticker: length %d", n)
	}
	out := make([]*InputWalletCardSticker, 0, n)
	for i := int32(0); i < n; i++ {
		row := &InputWalletCardSticker{}
		if err := row.Decode(dBuf); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, dBuf.GetError()
}

func decodeCardStickers(dBuf *DecodeBuf) ([]*Wallet_CardSticker, error) {
	c := dBuf.Int()
	if c != int32(CRC32_vector) {
		return nil, fmt.Errorf("wallet.cardSticker: expected vector, got 0x%x", uint32(c))
	}
	n := dBuf.Int()
	if n < 0 || n > maxVectorLen {
		return nil, fmt.Errorf("wallet.cardSticker: length %d", n)
	}
	out := make([]*Wallet_CardSticker, 0, n)
	for i := int32(0); i < n; i++ {
		row := &Wallet_CardSticker{}
		if err := row.Decode(dBuf); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, dBuf.GetError()
}
