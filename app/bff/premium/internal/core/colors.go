package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// nameColors are help.getPeerColors entries. Ids start at 0, which official
// clients treat as the first name color. Values are sRGB.
var nameColors = [][2]int32{
	{0xe17076, 0xf17a80},
	{0xfaa05a, 0xffb264},
	{0xa695e7, 0xb4a4f0},
	{0x7bc862, 0x8ad470},
	{0x6ec9cb, 0x7cd4d6},
	{0x65aadd, 0x74b6e8},
	{0xee7aae, 0xf589bb},
	{0xe5ca77, 0xf0d585},
}

// profileColors are help.getPeerProfileColors entries.
var profileColors = [][3]int32{
	{0xe17076, 0xf8d2d4, 0xffffff},
	{0xfaa05a, 0xfde3c8, 0xffffff},
	{0xa695e7, 0xe4def8, 0xffffff},
	{0x7bc862, 0xd7f0cf, 0xffffff},
	{0x65aadd, 0xd3e8f6, 0xffffff},
	{0xee7aae, 0xfad4e6, 0xffffff},
}

func peerColors(profile bool) *mtproto.Help_PeerColors {
	var options []*mtproto.Help_PeerColorOption
	if profile {
		for id, row := range profileColors {
			options = append(options, mtproto.MakeTLHelpPeerColorOption(&mtproto.Help_PeerColorOption{
				ColorId: int32(id),
				Colors: mtproto.MakeTLHelpPeerColorProfileSet(&mtproto.Help_PeerColorSet{
					PaletteColors: []int32{row[0], row[1]},
					BgColors:      []int32{row[1], row[2]},
					StoryColors:   []int32{row[0], row[1]},
				}).To_Help_PeerColorSet(),
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
