package p1rest

import "github.com/teamgram/proto/mtproto"

func backdropCatalog() []*mtproto.ShareThemeBackdropOption {
	type row struct {
		id                          int32
		name                        string
		center, edge, pattern, text int32
	}
	raw := []row{
		{1, "Onyx", 0x0e0e10, 0x1c1c22, 0x6b6b75, 0xffffff},
		{2, "Midnight", 0x0b1d36, 0x123056, 0x8fb4ff, 0xffffff},
		{3, "Emerald", 0x0d3b2e, 0x146b4a, 0x9dffc8, 0xffffff},
		{4, "Ruby", 0x4a1020, 0x7a1b33, 0xffb4c4, 0xffffff},
	}
	out := make([]*mtproto.ShareThemeBackdropOption, 0, len(raw))
	for _, r := range raw {
		out = append(out, &mtproto.ShareThemeBackdropOption{
			PredicateName: mtproto.Predicate_shareThemeBackdropOption,
			BackdropId:    r.id,
			Name:          r.name,
			CenterColor:   r.center,
			EdgeColor:     r.edge,
			PatternColor:  r.pattern,
			TextColor:     r.text,
		})
	}
	return out
}

func backdropHash(rows []*mtproto.ShareThemeBackdropOption) int32 {
	var h int32 = 17
	for _, row := range rows {
		h = h*31 + row.GetBackdropId()
		h = h*31 + row.GetCenterColor()
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		h = 1
	}
	return h
}

func (s *store) setShareTheme(self int64, peer *mtproto.InputPeer, theme *mtproto.ShareTheme) (*mtproto.Bool, error) {
	ref, _, err := peerOf(self, peer)
	if err != nil {
		return nil, err
	}
	label, err := shareLabel(theme)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	k := dialogKey{user: self, kind: ref.kind, id: ref.id}
	if label == "" {
		delete(s.shares, k)
	} else {
		s.shares[k] = label
	}
	s.mu.Unlock()
	return mtproto.BoolTrue, nil
}

func shareLabel(theme *mtproto.ShareTheme) (string, error) {
	if theme == nil || theme.GetPredicateName() == "" {
		return "", mtproto.ErrThemeInvalid
	}
	switch theme.GetPredicateName() {
	case mtproto.Predicate_shareThemeDefault:
		return "", nil
	case mtproto.Predicate_shareThemeEmojiBackdrop:
		if !theme.GetHasBackdrop() && len(theme.GetEmojis()) == 0 {
			return "", mtproto.ErrThemeInvalid
		}
		return "emoji", nil
	case mtproto.Predicate_shareThemeGift:
		if theme.GetStargift() == nil {
			return "", mtproto.ErrThemeInvalid
		}
		return "gift", nil
	case mtproto.Predicate_inputShareThemeGift:
		if theme.GetInputGift() == nil {
			return "", mtproto.ErrThemeInvalid
		}
		return "input-gift", nil
	default:
		return "", mtproto.ErrThemeInvalid
	}
}

func shareBackdrops(hash int32) *mtproto.ShareThemeBackdrops {
	rows := backdropCatalog()
	sum := backdropHash(rows)
	if hash != 0 && hash == sum {
		return mtproto.MakeShareThemeBackdropsNotModified()
	}
	return mtproto.MakeShareThemeBackdrops(sum, rows)
}
