package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// nameColors are help.getPeerColors entries. Ids start at 0. The first value
// is the light-theme foreground; the second is the lighter dark-theme color.
// These match the saturated red/orange/violet/green/cyan/blue/pink set
// official clients use for message accents.
var nameColors = [][2]int32{
	{0xcc5049, 0xd5766f},
	{0xd67722, 0xe08a3c},
	{0x955cdb, 0xa87ae4},
	{0x40a920, 0x5cbf3a},
	{0x309eba, 0x48b4cc},
	{0x368ad1, 0x4ea0de},
	{0xc7508b, 0xd46aa0},
	{0xc5a024, 0xd4b44a},
}

// profilePalette is one help.peerColorProfileSet plus its dark_colors twin.
// Backgrounds are saturated. A fade to white makes clients draw white
// buttons and the name on a white header.
type profilePalette struct {
	palette [2]int32
	bg      [2]int32
	story   [2]int32
	darkPal [2]int32
	darkBg  [2]int32
	darkSt  [2]int32
}

var profileColors = []profilePalette{
	{palette: [2]int32{0xc45058, 0x9b3a42}, bg: [2]int32{0x9b3a42, 0xc45058}, story: [2]int32{0xc45058, 0x9b3a42}, darkPal: [2]int32{0x8c343c, 0x6e242c}, darkBg: [2]int32{0x6e242c, 0x8c343c}, darkSt: [2]int32{0x8c343c, 0x6e242c}},
	{palette: [2]int32{0xd4842a, 0xb86a12}, bg: [2]int32{0xb86a12, 0xd4842a}, story: [2]int32{0xd4842a, 0xb86a12}, darkPal: [2]int32{0x9a5a14, 0x7a440c}, darkBg: [2]int32{0x7a440c, 0x9a5a14}, darkSt: [2]int32{0x9a5a14, 0x7a440c}},
	{palette: [2]int32{0x8660c8, 0x6a45b0}, bg: [2]int32{0x6a45b0, 0x8660c8}, story: [2]int32{0x8660c8, 0x6a45b0}, darkPal: [2]int32{0x62409a, 0x4a2e80}, darkBg: [2]int32{0x4a2e80, 0x62409a}, darkSt: [2]int32{0x62409a, 0x4a2e80}},
	{palette: [2]int32{0x46a84a, 0x2f8a32}, bg: [2]int32{0x2f8a32, 0x46a84a}, story: [2]int32{0x46a84a, 0x2f8a32}, darkPal: [2]int32{0x2c7630, 0x1c5e20}, darkBg: [2]int32{0x1c5e20, 0x2c7630}, darkSt: [2]int32{0x2c7630, 0x1c5e20}},
	{palette: [2]int32{0x2a96a6, 0x1c7c8c}, bg: [2]int32{0x1c7c8c, 0x2a96a6}, story: [2]int32{0x2a96a6, 0x1c7c8c}, darkPal: [2]int32{0x187080, 0x105660}, darkBg: [2]int32{0x105660, 0x187080}, darkSt: [2]int32{0x187080, 0x105660}},
	{palette: [2]int32{0x3c7cc8, 0x2a62b0}, bg: [2]int32{0x2a62b0, 0x3c7cc8}, story: [2]int32{0x3c7cc8, 0x2a62b0}, darkPal: [2]int32{0x245898, 0x1a4480}, darkBg: [2]int32{0x1a4480, 0x245898}, darkSt: [2]int32{0x245898, 0x1a4480}},
	{palette: [2]int32{0xc85c90, 0xb04478}, bg: [2]int32{0xb04478, 0xc85c90}, story: [2]int32{0xc85c90, 0xb04478}, darkPal: [2]int32{0x9a3c68, 0x7c2c52}, darkBg: [2]int32{0x7c2c52, 0x9a3c68}, darkSt: [2]int32{0x9a3c68, 0x7c2c52}},
	{palette: [2]int32{0xc09628, 0xa07c14}, bg: [2]int32{0xa07c14, 0xc09628}, story: [2]int32{0xc09628, 0xa07c14}, darkPal: [2]int32{0x8a6a14, 0x6c520c}, darkBg: [2]int32{0x6c520c, 0x8a6a14}, darkSt: [2]int32{0x8a6a14, 0x6c520c}},
}

func profileSet(palette, bg, story [2]int32) *mtproto.Help_PeerColorSet {
	return mtproto.MakeTLHelpPeerColorProfileSet(&mtproto.Help_PeerColorSet{
		PaletteColors: []int32{palette[0], palette[1]},
		BgColors:      []int32{bg[0], bg[1]},
		StoryColors:   []int32{story[0], story[1]},
	}).To_Help_PeerColorSet()
}

func peerColors(profile bool) *mtproto.Help_PeerColors {
	var options []*mtproto.Help_PeerColorOption
	if profile {
		for id, row := range profileColors {
			options = append(options, mtproto.MakeTLHelpPeerColorOption(&mtproto.Help_PeerColorOption{
				ColorId:    int32(id),
				Colors:     profileSet(row.palette, row.bg, row.story),
				DarkColors: profileSet(row.darkPal, row.darkBg, row.darkSt),
			}).To_Help_PeerColorOption())
		}
	} else {
		for id, row := range nameColors {
			options = append(options, mtproto.MakeTLHelpPeerColorOption(&mtproto.Help_PeerColorOption{
				ColorId: int32(id),
				Colors: mtproto.MakeTLHelpPeerColorSet(&mtproto.Help_PeerColorSet{
					Colors: []int32{row[0]},
				}).To_Help_PeerColorSet(),
				DarkColors: mtproto.MakeTLHelpPeerColorSet(&mtproto.Help_PeerColorSet{
					Colors: []int32{row[1]},
				}).To_Help_PeerColorSet(),
			}).To_Help_PeerColorOption())
		}
	}
	return mtproto.MakeTLHelpPeerColors(&mtproto.Help_PeerColors{
		Hash:   premium.PeerColorHash,
		Colors: options,
	}).To_Help_PeerColors()
}

func peerColorsReply(hash int32, profile bool) *mtproto.Help_PeerColors {
	if hash == premium.PeerColorHash {
		return mtproto.MakeTLHelpPeerColorsNotModified(&mtproto.Help_PeerColors{}).To_Help_PeerColors()
	}
	return peerColors(profile)
}

// knownPeerColor reports whether id is one of the colors help.getPeerColors
// or help.getPeerProfileColors returns. The two lists are separate.
func knownPeerColor(profile bool, id int32) bool {
	for _, opt := range peerColors(profile).GetColors() {
		if opt.GetColorId() == id {
			return true
		}
	}
	return false
}

// selectedColor reads account.updateColor. A missing color flag clears the color.
func selectedColor(in *mtproto.TLAccountUpdateColor) (colorID int32, emoji int64, hasColor bool) {
	colorID = -1
	if in == nil {
		return colorID, 0, false
	}
	if peer := in.GetColor_FLAGPEERCOLOR(); peer != nil {
		if peer.GetColor() != nil {
			colorID = peer.GetColor().GetValue()
			hasColor = true
		}
		if peer.GetBackgroundEmojiId() != nil {
			emoji = peer.GetBackgroundEmojiId().GetValue()
		}
	}
	if wrapped := in.GetColor_FLAGINT32(); wrapped != nil {
		colorID = wrapped.GetValue()
		hasColor = true
	}
	if in.GetBackgroundEmojiId() != nil {
		emoji = in.GetBackgroundEmojiId().GetValue()
	}
	return colorID, emoji, hasColor
}

func emojiStatusParts(st *mtproto.EmojiStatus) (documentID int64, until int32) {
	if st == nil || st.GetPredicateName() == mtproto.Predicate_emojiStatusEmpty {
		return 0, 0
	}
	until = st.GetUntil_INT32()
	if st.GetUntil_FLAGINT32() != nil {
		until = st.GetUntil_FLAGINT32().GetValue()
	}
	return st.GetDocumentId(), until
}
