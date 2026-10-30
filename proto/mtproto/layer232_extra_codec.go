package mtproto

import "fmt"

// Layer 232 share-theme constructors and the Android messages.getStatsURL
// body. Share-theme types are in telegram_api_layer232.tl. getStatsURL is
// not: Android 3de37c8 still sends 0x812c2ae6 with a flags body.
//
//	shareThemeDefault#95ace84f = ShareTheme;
//	shareThemeEmojiBackdrop#64b334e1 flags:# backdrop_id:flags.0?int emojis:flags.1?Vector<Reaction> = ShareTheme;
//	shareThemeGift#ebece66d stargift:StarGift = ShareTheme;
//	inputShareThemeGift#3fb92b5b stargift:InputSavedStarGift = ShareTheme;
//	shareThemeBackdropOption#45c1d652 backdrop_id:int name:string center_color:int edge_color:int pattern_color:int text_color:int = ShareThemeBackdropOption;
//	shareThemeBackdropsNotModified#7a3f6dff = ShareThemeBackdrops;
//	shareThemeBackdrops#aaeadce0 hash:int backdrops:Vector<ShareThemeBackdropOption> = ShareThemeBackdrops;
//	messages.setShareTheme#698a9b1a peer:InputPeer theme:ShareTheme = Bool;
//	messages.getShareThemeBackdrops#f4599ef2 hash:int = ShareThemeBackdrops;
//	messages.getStatsURL#812c2ae6 flags:# dark:flags.0?true peer:InputPeer params:string = StatsURL;
const (
	Predicate_shareThemeDefault                      = "shareThemeDefault"
	Predicate_shareThemeEmojiBackdrop                = "shareThemeEmojiBackdrop"
	Predicate_shareThemeGift                         = "shareThemeGift"
	Predicate_inputShareThemeGift                    = "inputShareThemeGift"
	Predicate_shareThemeBackdropOption               = "shareThemeBackdropOption"
	Predicate_shareThemeBackdropsNotModified         = "shareThemeBackdropsNotModified"
	Predicate_shareThemeBackdrops                    = "shareThemeBackdrops"
	Predicate_messages_setShareTheme                 = "messages_setShareTheme"
	Predicate_messages_getShareThemeBackdrops        = "messages_getShareThemeBackdrops"
	Predicate_messages_getStatsURL                   = "messages_getStatsURL"
	crcShareThemeDefault                      uint32 = 0x95ace84f
	crcShareThemeEmojiBackdrop                uint32 = 0x64b334e1
	crcShareThemeGift                         uint32 = 0xebece66d
	crcInputShareThemeGift                    uint32 = 0x3fb92b5b
	crcShareThemeBackdropOption               uint32 = 0x45c1d652
	crcShareThemeBackdropsNotModified         uint32 = 0x7a3f6dff
	crcShareThemeBackdrops                    uint32 = 0xaaeadce0
	crcMessagesSetShareTheme                  uint32 = 0x698a9b1a
	crcMessagesGetShareThemeBackdrops         uint32 = 0xf4599ef2
	crcMessagesGetStatsURL                    uint32 = 0x812c2ae6
)

func init() {
	registerLayer232Extra(Predicate_shareThemeDefault, crcShareThemeDefault, func(c int32) TLObject {
		return &ShareTheme{PredicateName: Predicate_shareThemeDefault, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_shareThemeEmojiBackdrop, crcShareThemeEmojiBackdrop, func(c int32) TLObject {
		return &ShareTheme{PredicateName: Predicate_shareThemeEmojiBackdrop, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_shareThemeGift, crcShareThemeGift, func(c int32) TLObject {
		return &ShareTheme{PredicateName: Predicate_shareThemeGift, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_inputShareThemeGift, crcInputShareThemeGift, func(c int32) TLObject {
		return &ShareTheme{PredicateName: Predicate_inputShareThemeGift, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_shareThemeBackdropOption, crcShareThemeBackdropOption, func(c int32) TLObject {
		return &ShareThemeBackdropOption{PredicateName: Predicate_shareThemeBackdropOption, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_shareThemeBackdropsNotModified, crcShareThemeBackdropsNotModified, func(c int32) TLObject {
		return &ShareThemeBackdrops{PredicateName: Predicate_shareThemeBackdropsNotModified, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_shareThemeBackdrops, crcShareThemeBackdrops, func(c int32) TLObject {
		return &ShareThemeBackdrops{PredicateName: Predicate_shareThemeBackdrops, Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_messages_setShareTheme, crcMessagesSetShareTheme, func(c int32) TLObject {
		return &TLMessagesSetShareTheme{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_messages_getShareThemeBackdrops, crcMessagesGetShareThemeBackdrops, func(c int32) TLObject {
		return &TLMessagesGetShareThemeBackdrops{Constructor: TLConstructor(c)}
	})
	registerLayer232Extra(Predicate_messages_getStatsURL, crcMessagesGetStatsURL, func(c int32) TLObject {
		return &TLMessagesGetStatsURL{Constructor: TLConstructor(c)}
	})
}

func ctorID(v uint32) TLConstructor { return TLConstructor(int32(v)) }

func registerLayer232Extra(name string, crc uint32, newFn func(int32) TLObject) {
	id := int32(crc)
	clazzNameRegisters2[name] = map[int]int32{0: id, 232: id}
	clazzIdNameRegisters2[id] = name
	clazzIdRegisters2[id] = func() TLObject { return newFn(id) }
}

// MakeShareThemeDefault builds shareThemeDefault.
func MakeShareThemeDefault() *ShareTheme {
	return &ShareTheme{
		PredicateName: Predicate_shareThemeDefault,
		Constructor:   ctorID(crcShareThemeDefault),
	}
}

// MakeShareThemeBackdrops builds shareThemeBackdrops.
func MakeShareThemeBackdrops(hash int32, rows []*ShareThemeBackdropOption) *ShareThemeBackdrops {
	if rows == nil {
		rows = []*ShareThemeBackdropOption{}
	}
	return &ShareThemeBackdrops{
		PredicateName: Predicate_shareThemeBackdrops,
		Constructor:   ctorID(crcShareThemeBackdrops),
		Hash:          hash,
		Backdrops:     rows,
	}
}

// MakeShareThemeBackdropsNotModified builds shareThemeBackdropsNotModified.
func MakeShareThemeBackdropsNotModified() *ShareThemeBackdrops {
	return &ShareThemeBackdrops{
		PredicateName: Predicate_shareThemeBackdropsNotModified,
		Constructor:   ctorID(crcShareThemeBackdropsNotModified),
	}
}

// Encode writes the ShareTheme constructor that matches PredicateName.
func (m *ShareTheme) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		m = MakeShareThemeDefault()
	}
	switch m.GetPredicateName() {
	case Predicate_shareThemeDefault, "":
		x.UInt(crcShareThemeDefault)
		return nil
	case Predicate_shareThemeEmojiBackdrop:
		x.UInt(crcShareThemeEmojiBackdrop)
		var flags uint32
		if m.GetHasBackdrop() {
			flags |= 1 << 0
		}
		if len(m.GetEmojis()) > 0 {
			flags |= 1 << 1
		}
		x.UInt(flags)
		if flags&(1<<0) != 0 {
			x.Int(m.GetBackdropId())
		}
		if flags&(1<<1) != 0 {
			if err := encodeReactions(x, layer, m.GetEmojis()); err != nil {
				return err
			}
		}
		return nil
	case Predicate_shareThemeGift:
		x.UInt(crcShareThemeGift)
		if m.GetStargift() == nil {
			return fmt.Errorf("shareThemeGift: missing stargift")
		}
		return m.GetStargift().Encode(x, layer)
	case Predicate_inputShareThemeGift:
		x.UInt(crcInputShareThemeGift)
		if m.GetInputGift() == nil {
			return fmt.Errorf("inputShareThemeGift: missing stargift")
		}
		return m.GetInputGift().Encode(x, layer)
	default:
		return fmt.Errorf("share theme: predicate %s", m.GetPredicateName())
	}
}

// Decode reads a ShareTheme. Object() has already consumed the constructor
// when Constructor is set; a nested field starts at Constructor 0.
func (m *ShareTheme) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	switch uint32(m.Constructor) {
	case crcShareThemeDefault:
		m.PredicateName = Predicate_shareThemeDefault
	case crcShareThemeEmojiBackdrop:
		m.PredicateName = Predicate_shareThemeEmojiBackdrop
		flags := dBuf.UInt()
		if flags&(1<<0) != 0 {
			m.HasBackdrop = true
			m.BackdropId = dBuf.Int()
		}
		if flags&(1<<1) != 0 {
			emojis, err := decodeReactions(dBuf)
			if err != nil {
				return err
			}
			m.Emojis = emojis
		}
	case crcShareThemeGift:
		m.PredicateName = Predicate_shareThemeGift
		gift := &StarGift{}
		if err := gift.Decode(dBuf); err != nil {
			return err
		}
		m.Stargift = gift
	case crcInputShareThemeGift:
		m.PredicateName = Predicate_inputShareThemeGift
		gift := &InputSavedStarGift{}
		if err := gift.Decode(dBuf); err != nil {
			return err
		}
		m.InputGift = gift
	default:
		return fmt.Errorf("share theme: constructor 0x%x", uint32(m.Constructor))
	}
	return dBuf.GetError()
}

// Encode writes shareThemeBackdropOption#45c1d652.
func (m *ShareThemeBackdropOption) Encode(x *EncodeBuf, layer int32) error {
	_ = layer
	if m == nil {
		m = &ShareThemeBackdropOption{}
	}
	x.UInt(crcShareThemeBackdropOption)
	x.Int(m.GetBackdropId())
	x.String(m.GetName())
	x.Int(m.GetCenterColor())
	x.Int(m.GetEdgeColor())
	x.Int(m.GetPatternColor())
	x.Int(m.GetTextColor())
	return nil
}

// Decode reads shareThemeBackdropOption. See ShareTheme.Decode for the constructor rule.
func (m *ShareThemeBackdropOption) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	if uint32(m.Constructor) != crcShareThemeBackdropOption {
		return fmt.Errorf("shareThemeBackdropOption: constructor 0x%x", uint32(m.Constructor))
	}
	m.PredicateName = Predicate_shareThemeBackdropOption
	m.BackdropId = dBuf.Int()
	m.Name = dBuf.String()
	m.CenterColor = dBuf.Int()
	m.EdgeColor = dBuf.Int()
	m.PatternColor = dBuf.Int()
	m.TextColor = dBuf.Int()
	return dBuf.GetError()
}

// Encode writes shareThemeBackdrops or shareThemeBackdropsNotModified.
func (m *ShareThemeBackdrops) Encode(x *EncodeBuf, layer int32) error {
	if m == nil || m.GetPredicateName() == Predicate_shareThemeBackdropsNotModified {
		x.UInt(crcShareThemeBackdropsNotModified)
		return nil
	}
	x.UInt(crcShareThemeBackdrops)
	x.Int(m.GetHash())
	rows := m.GetBackdrops()
	x.Int(int32(CRC32_vector))
	x.Int(int32(len(rows)))
	for _, row := range rows {
		if err := row.Encode(x, layer); err != nil {
			return err
		}
	}
	return nil
}

// Decode reads ShareThemeBackdrops. See ShareTheme.Decode for the constructor rule.
func (m *ShareThemeBackdrops) Decode(dBuf *DecodeBuf) error {
	if m.Constructor == 0 {
		m.Constructor = TLConstructor(dBuf.Int())
	}
	switch uint32(m.Constructor) {
	case crcShareThemeBackdropsNotModified:
		m.PredicateName = Predicate_shareThemeBackdropsNotModified
	case crcShareThemeBackdrops:
		m.PredicateName = Predicate_shareThemeBackdrops
		m.Hash = dBuf.Int()
		c := dBuf.Int()
		if c != int32(CRC32_vector) {
			return fmt.Errorf("shareThemeBackdrops: expected vector, got 0x%x", uint32(c))
		}
		n := dBuf.Int()
		if n < 0 || n > maxVectorLen {
			return fmt.Errorf("shareThemeBackdrops: length %d", n)
		}
		m.Backdrops = make([]*ShareThemeBackdropOption, 0, n)
		for i := int32(0); i < n; i++ {
			row := &ShareThemeBackdropOption{}
			if err := row.Decode(dBuf); err != nil {
				return err
			}
			m.Backdrops = append(m.Backdrops, row)
		}
	default:
		return fmt.Errorf("shareThemeBackdrops: constructor 0x%x", uint32(m.Constructor))
	}
	return dBuf.GetError()
}

// Encode writes messages.setShareTheme#698a9b1a.
func (m *TLMessagesSetShareTheme) Encode(x *EncodeBuf, layer int32) error {
	x.UInt(crcMessagesSetShareTheme)
	if m.GetPeer() == nil {
		if err := MakeTLInputPeerEmpty(nil).To_InputPeer().Encode(x, layer); err != nil {
			return err
		}
	} else if err := m.GetPeer().Encode(x, layer); err != nil {
		return err
	}
	return m.GetTheme().Encode(x, layer)
}

// Decode reads peer and theme. Object() has already consumed the constructor.
func (m *TLMessagesSetShareTheme) Decode(dBuf *DecodeBuf) error {
	peer := &InputPeer{}
	if err := peer.Decode(dBuf); err != nil {
		return err
	}
	m.Peer = peer
	theme := &ShareTheme{}
	if err := theme.Decode(dBuf); err != nil {
		return err
	}
	m.Theme = theme
	return dBuf.GetError()
}

// Encode writes messages.getShareThemeBackdrops#f4599ef2.
func (m *TLMessagesGetShareThemeBackdrops) Encode(x *EncodeBuf, layer int32) error {
	_ = layer
	x.UInt(crcMessagesGetShareThemeBackdrops)
	x.Int(m.GetHash())
	return nil
}

// Decode reads hash. Object() has already consumed the constructor.
func (m *TLMessagesGetShareThemeBackdrops) Decode(dBuf *DecodeBuf) error {
	m.Hash = dBuf.Int()
	return dBuf.GetError()
}

// Encode writes messages.getStatsURL#812c2ae6.
func (m *TLMessagesGetStatsURL) Encode(x *EncodeBuf, layer int32) error {
	x.UInt(crcMessagesGetStatsURL)
	var flags uint32
	if m.GetDark() {
		flags |= 1 << 0
	}
	x.UInt(flags)
	if m.GetPeer() == nil {
		if err := MakeTLInputPeerEmpty(nil).To_InputPeer().Encode(x, layer); err != nil {
			return err
		}
	} else if err := m.GetPeer().Encode(x, layer); err != nil {
		return err
	}
	x.String(m.GetParams())
	return nil
}

// Decode reads flags, peer, and params. Object() has already consumed the constructor.
func (m *TLMessagesGetStatsURL) Decode(dBuf *DecodeBuf) error {
	flags := dBuf.UInt()
	m.Dark = flags&(1<<0) != 0
	peer := &InputPeer{}
	if err := peer.Decode(dBuf); err != nil {
		return err
	}
	m.Peer = peer
	m.Params = dBuf.String()
	return dBuf.GetError()
}

func encodeReactions(x *EncodeBuf, layer int32, rows []*Reaction) error {
	x.Int(int32(CRC32_vector))
	x.Int(int32(len(rows)))
	for _, row := range rows {
		if row == nil {
			return fmt.Errorf("share theme: nil reaction")
		}
		if err := row.Encode(x, layer); err != nil {
			return err
		}
	}
	return nil
}

func decodeReactions(dBuf *DecodeBuf) ([]*Reaction, error) {
	c := dBuf.Int()
	if c != int32(CRC32_vector) {
		return nil, fmt.Errorf("share theme emojis: expected vector, got 0x%x", uint32(c))
	}
	n := dBuf.Int()
	if n < 0 || n > maxVectorLen {
		return nil, fmt.Errorf("share theme emojis: length %d", n)
	}
	out := make([]*Reaction, 0, n)
	for i := int32(0); i < n; i++ {
		row := &Reaction{}
		if err := row.Decode(dBuf); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, dBuf.GetError()
}
