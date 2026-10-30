package mtproto

import "fmt"

// Host gift constructors are in telegram_api_layer232.tl but were not emitted
// by the vendored layer-231 codegen. See schema.tl.stargift_host.proto.
const (
	Predicate_payments_starGiftDepositPayload        = "payments_starGiftDepositPayload"
	Predicate_payments_deleteStarGift                = "payments_deleteStarGift"
	Predicate_payments_withdrawStarGift              = "payments_withdrawStarGift"
	Predicate_payments_depositStarGift               = "payments_depositStarGift"
	Predicate_payments_displayStarGift               = "payments_displayStarGift"
	crcStarGiftDepositPayload                 uint32 = 0x725bb4eb
	crcPaymentsDeleteStarGift                 uint32 = 0x3cdaf8aa
	crcPaymentsWithdrawStarGift               uint32 = 0xec2030ae
	crcPaymentsDepositStarGift                uint32 = 0xc0a2ea9f
	crcPaymentsDisplayStarGift                uint32 = 0x4aa5f0fe
)

func init() {
	registerLayer232Extra(Predicate_payments_starGiftDepositPayload, crcStarGiftDepositPayload, func(c int32) TLObject {
		return &Payments_StarGiftDepositPayload{PredicateName: Predicate_payments_starGiftDepositPayload, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_payments_deleteStarGift, crcPaymentsDeleteStarGift, func(c int32) TLObject {
		return &TLPaymentsDeleteStarGift{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_payments_withdrawStarGift, crcPaymentsWithdrawStarGift, func(c int32) TLObject {
		return &TLPaymentsWithdrawStarGift{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_payments_depositStarGift, crcPaymentsDepositStarGift, func(c int32) TLObject {
		return &TLPaymentsDepositStarGift{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_payments_displayStarGift, crcPaymentsDisplayStarGift, func(c int32) TLObject {
		return &TLPaymentsDisplayStarGift{Constructor: TLConstructor(c)}
	})
}

func (m *Payments_StarGiftDepositPayload) Encode(x *EncodeBuf, layer int32) error {
	_ = layer
	if m == nil {
		return fmt.Errorf("payments.starGiftDepositPayload: nil")
	}
	x.UInt(crcStarGiftDepositPayload)
	x.String(m.GetNftAddress())
	x.StringBytes(m.GetPayload())
	x.Long(m.GetAmount())
	return nil
}

func (m *Payments_StarGiftDepositPayload) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcStarGiftDepositPayload {
		return fmt.Errorf("payments.starGiftDepositPayload: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_payments_starGiftDepositPayload
	m.NftAddress = dBuf.String()
	m.Payload = dBuf.StringBytes()
	m.Amount = dBuf.Long()
	return dBuf.GetError()
}

func (m *TLPaymentsDeleteStarGift) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLPaymentsDeleteStarGift{}
	}
	x.UInt(crcPaymentsDeleteStarGift)
	return encodeSavedGift(x, layer, m.GetStargift())
}

func (m *TLPaymentsDeleteStarGift) Decode(dBuf *DecodeBuf) error {
	gift, err := decodeSavedGift(dBuf)
	if err != nil {
		return err
	}
	m.Stargift = gift
	return dBuf.GetError()
}

func (m *TLPaymentsWithdrawStarGift) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLPaymentsWithdrawStarGift{}
	}
	x.UInt(crcPaymentsWithdrawStarGift)
	var flags uint32
	if m.GetKeepOnProfile() {
		flags |= 1 << 0
	}
	x.UInt(flags)
	if err := encodeSavedGift(x, layer, m.GetStargift()); err != nil {
		return err
	}
	x.StringBytes(m.GetTmpPassword())
	return nil
}

func (m *TLPaymentsWithdrawStarGift) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	m.KeepOnProfile = flags&(1<<0) != 0
	gift, err := decodeSavedGift(dBuf)
	if err != nil {
		return err
	}
	m.Stargift = gift
	m.TmpPassword = dBuf.StringBytes()
	return dBuf.GetError()
}

func (m *TLPaymentsDepositStarGift) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLPaymentsDepositStarGift{}
	}
	x.UInt(crcPaymentsDepositStarGift)
	var flags uint32
	if m.GetHideSender() {
		flags |= 1 << 0
	}
	x.UInt(flags)
	if err := encodeInputPeer(x, layer, m.GetPeer()); err != nil {
		return err
	}
	x.String(m.GetSlug())
	return nil
}

func (m *TLPaymentsDepositStarGift) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	m.HideSender = flags&(1<<0) != 0
	peer, err := decodeInputPeer(dBuf)
	if err != nil {
		return err
	}
	m.Peer = peer
	m.Slug = dBuf.String()
	return dBuf.GetError()
}

func (m *TLPaymentsDisplayStarGift) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = &TLPaymentsDisplayStarGift{}
	}
	x.UInt(crcPaymentsDisplayStarGift)
	var flags uint32
	if m.GetHasPeer() && m.GetPeer() != nil {
		flags |= 1 << 0
	}
	x.UInt(flags)
	if flags&(1<<0) != 0 {
		if err := encodeInputPeer(x, layer, m.GetPeer()); err != nil {
			return err
		}
	}
	x.String(m.GetSlug())
	return nil
}

func (m *TLPaymentsDisplayStarGift) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	m.HasPeer = flags&(1<<0) != 0
	if m.HasPeer {
		peer, err := decodeInputPeer(dBuf)
		if err != nil {
			return err
		}
		m.Peer = peer
	}
	m.Slug = dBuf.String()
	return dBuf.GetError()
}

func encodeSavedGift(x *EncodeBuf, layer int32, gift *InputSavedStarGift) error {
	if gift == nil {
		return fmt.Errorf("payments: missing InputSavedStarGift")
	}
	return gift.Encode(x, layer)
}

func decodeSavedGift(dBuf *DecodeBuf) (*InputSavedStarGift, error) {
	obj := dBuf.Object()
	if err := dBuf.GetError(); err != nil {
		return nil, err
	}
	switch g := obj.(type) {
	case *InputSavedStarGift:
		return g, nil
	case interface {
		To_InputSavedStarGift() *InputSavedStarGift
	}:
		return g.To_InputSavedStarGift(), nil
	default:
		return nil, fmt.Errorf("payments: InputSavedStarGift")
	}
}

func encodeInputPeer(x *EncodeBuf, layer int32, peer *InputPeer) error {
	if peer == nil {
		return fmt.Errorf("payments: missing InputPeer")
	}
	return peer.Encode(x, layer)
}

func decodeInputPeer(dBuf *DecodeBuf) (*InputPeer, error) {
	obj := dBuf.Object()
	if err := dBuf.GetError(); err != nil {
		return nil, err
	}
	switch p := obj.(type) {
	case *InputPeer:
		return p, nil
	case interface{ To_InputPeer() *InputPeer }:
		return p.To_InputPeer(), nil
	default:
		return nil, fmt.Errorf("payments: InputPeer")
	}
}
