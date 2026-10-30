package mtproto

import "testing"

func TestShareThemeAndStatsURLRoundTrip(t *testing.T) {
	if got := uint32(GetClazzID(Predicate_messages_setShareTheme, Layer)); got != crcMessagesSetShareTheme {
		t.Fatalf("setShareTheme ctor 0x%x", got)
	}
	if got := uint32(GetClazzID(Predicate_messages_getStatsURL, Layer)); got != crcMessagesGetStatsURL {
		t.Fatalf("getStatsURL ctor 0x%x", got)
	}

	req := &TLMessagesSetShareTheme{
		Peer:  MakeTLInputPeerUser(&InputPeer{UserId: 9, AccessHash: 3}).To_InputPeer(),
		Theme: MakeShareThemeDefault(),
	}
	raw := encodeAt(t, req, Layer)
	if ctorOf(raw) != crcMessagesSetShareTheme {
		t.Fatalf("request ctor 0x%x", ctorOf(raw))
	}
	got, ok := NewDecodeBuf(raw).Object().(*TLMessagesSetShareTheme)
	if !ok || got.GetPeer().GetUserId() != 9 || got.GetTheme().GetPredicateName() != Predicate_shareThemeDefault {
		t.Fatalf("setShareTheme %#v", got)
	}

	emoji := &ShareTheme{
		PredicateName: Predicate_shareThemeEmojiBackdrop,
		HasBackdrop:   true,
		BackdropId:    2,
		Emojis:        []*Reaction{MakeTLReactionEmoji(&Reaction{Emoticon: "🎄"}).To_Reaction()},
	}
	raw = encodeAt(t, emoji, Layer)
	if ctorOf(raw) != crcShareThemeEmojiBackdrop {
		t.Fatalf("emoji ctor 0x%x", ctorOf(raw))
	}
	gotEmoji, ok := NewDecodeBuf(raw).Object().(*ShareTheme)
	if !ok || !gotEmoji.GetHasBackdrop() || gotEmoji.GetBackdropId() != 2 || len(gotEmoji.GetEmojis()) != 1 || gotEmoji.Emojis[0].GetEmoticon() != "🎄" {
		t.Fatalf("emoji %#v", gotEmoji)
	}

	stats := &TLMessagesGetStatsURL{
		Dark:   true,
		Peer:   MakeTLInputPeerChannel(&InputPeer{ChannelId: 4, AccessHash: 8}).To_InputPeer(),
		Params: "refresh",
	}
	raw = encodeAt(t, stats, Layer)
	if ctorOf(raw) != 0x812c2ae6 {
		t.Fatalf("stats ctor 0x%x", ctorOf(raw))
	}
	gotStats, ok := NewDecodeBuf(raw).Object().(*TLMessagesGetStatsURL)
	if !ok || !gotStats.GetDark() || gotStats.GetParams() != "refresh" || gotStats.GetPeer().GetChannelId() != 4 {
		t.Fatalf("stats %#v peer %#v", gotStats, gotStats.GetPeer())
	}

	rows := MakeShareThemeBackdrops(7, []*ShareThemeBackdropOption{{
		PredicateName: Predicate_shareThemeBackdropOption,
		BackdropId:    1,
		Name:          "Onyx",
		CenterColor:   1,
		EdgeColor:     2,
		PatternColor:  3,
		TextColor:     4,
	}})
	raw = encodeAt(t, rows, Layer)
	gotRows, ok := NewDecodeBuf(raw).Object().(*ShareThemeBackdrops)
	if !ok || gotRows.GetHash() != 7 || len(gotRows.GetBackdrops()) != 1 || gotRows.Backdrops[0].GetName() != "Onyx" {
		t.Fatalf("rows %#v", gotRows)
	}
	raw = encodeAt(t, MakeShareThemeBackdropsNotModified(), Layer)
	if ctorOf(raw) != crcShareThemeBackdropsNotModified {
		t.Fatalf("not modified 0x%x", ctorOf(raw))
	}
}
