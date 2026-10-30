package catalog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
)

func TestBlocklistDropsTaiwanFlagFromListings(t *testing.T) {
	flag := emojiblock.TaiwanFlag
	grin := emojiFromCode("1f600")
	like := emojiFromCode("1f44d")
	flagItem := stickerItem(11, flag, false)
	grinItem := stickerItem(12, grin, false)
	flagEmoji := stickerItem(21, flag, true)
	grinEmoji := stickerItem(22, grin, true)
	onlyFlag := stickerItem(31, flag, true)

	s := newService([]*Pack{
		newPack("Flags", "Flags", KindSticker, false, false, true, true, "", "", "", []*Item{flagItem, grinItem}),
		newPack("FlagEmoji", "Flag Emoji", KindCustomEmoji, true, false, true, true, "", "", "", []*Item{flagEmoji, grinEmoji}),
		newPack("OnlyFlag", "Only Flag", KindCustomEmoji, true, false, true, true, "", "", "", []*Item{onlyFlag}),
	})
	const user int64 = 3

	all := s.AllStickers(user, 0)
	if len(all.GetSets()) != 1 || all.GetSets()[0].GetShortName() != "Flags" || all.GetSets()[0].GetCount() != 1 {
		t.Fatalf("all stickers %#v", all.GetSets())
	}
	emojiSets := s.EmojiStickers(user, 0)
	if len(emojiSets.GetSets()) != 1 || emojiSets.GetSets()[0].GetShortName() != "FlagEmoji" || emojiSets.GetSets()[0].GetCount() != 1 {
		t.Fatalf("emoji stickers %#v", emojiSets.GetSets())
	}

	got, err := s.GetSet(user, shortName("Flags"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDocuments()) != 1 || docAlt(got.GetDocuments()[0]) != grin {
		t.Fatalf("sticker documents %#v", alts(got.GetDocuments()))
	}
	if len(got.GetPacks()) != 1 || got.GetPacks()[0].GetEmoticon() != grin {
		t.Fatalf("sticker packs %#v", got.GetPacks())
	}
	if again, err := s.GetSet(user, shortName("Flags"), got.GetSet().GetHash()); err != nil || again.GetPredicateName() != mtproto.Predicate_messages_stickerSetNotModified {
		t.Fatalf("filtered set hash %#v %v", again, err)
	}

	featured := s.Featured(0, 0, true)
	if len(featured.GetSets()) != 1 || docAlt(featured.GetSets()[0].GetCover()) != grin {
		t.Fatalf("featured cover %#v", featured.GetSets())
	}
	if s.StickersByEmoji(flag, 0, false).GetStickers() == nil || len(s.StickersByEmoji(flag, 0, false).GetStickers()) != 0 {
		t.Fatal("blocked emoticon still returned stickers")
	}
	if len(s.StickersByEmoji(grin, 0, true).GetStickers()) != 1 {
		t.Fatal("unblocked custom emoji was dropped")
	}
	found := s.SearchDocuments(flag, true, 0)
	if len(found.GetStickers()) != 0 {
		t.Fatalf("search returned %#v", alts(found.GetStickers()))
	}
	if ids := s.CustomEmojiIDs(flag); len(ids) != 0 {
		t.Fatalf("custom emoji ids %#v", ids)
	}
	docs := s.Documents([]int64{flagEmoji.DocumentID, grinEmoji.DocumentID})
	if docs.Datas[0].GetPredicateName() != mtproto.Predicate_documentEmpty || docAlt(docs.Datas[1]) != grin {
		t.Fatalf("custom emoji documents %#v", docs.Datas)
	}

	if err = s.SaveRecent(user, flagItem.DocumentID, false, false); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRecent(user, grinItem.DocumentID, false, false); err != nil {
		t.Fatal(err)
	}
	recent := s.Recent(user, 0, false)
	if len(recent.GetStickers()) != 1 || docAlt(recent.GetStickers()[0]) != grin {
		t.Fatalf("recent %#v", alts(recent.GetStickers()))
	}
	if err = s.Fave(user, flagItem.DocumentID, false); err != nil {
		t.Fatal(err)
	}
	if len(s.Faved(user, 0).GetStickers()) != 0 {
		t.Fatal("faved list included the blocked sticker")
	}

	only, err := s.GetSet(user, shortName("OnlyFlag"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if only.GetSet().GetCount() != 0 || len(only.GetDocuments()) != 0 {
		t.Fatalf("fully blocked set %#v", only.GetSet())
	}

	s.NoteReaction(user, []string{flag, like})
	reactions := s.RecentReactions(user, 10, 0)
	if len(reactions.GetReactions()) != 1 || reactions.GetReactions()[0].GetEmoticon() != like {
		t.Fatalf("recent reactions %#v", reactions.GetReactions())
	}
	for _, reaction := range s.Reactions(0).GetReactions() {
		if reaction.GetReaction() == flag {
			t.Fatal("available reactions included the taiwan flag")
		}
	}
	for _, reaction := range s.TopReactions(0, 0).GetReactions() {
		if reaction.GetEmoticon() == flag {
			t.Fatal("top reactions included the taiwan flag")
		}
	}
}

func TestBlocklistExtraDropsCatalogEmoji(t *testing.T) {
	t.Cleanup(func() { emojiblock.Reload() })
	s := newService(builtinPacks())
	const user int64 = 4
	grin := emojiFromCode("1f600")
	before := s.AllStickers(user, 0).GetHash()
	setBefore, err := s.GetSet(user, shortName(ShortTwemoji), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAlt(setBefore.GetDocuments(), grin) {
		t.Fatal("twemoji is missing the grinning face")
	}

	t.Setenv(emojiblock.EnvExtra, "1F600")
	t.Setenv(emojiblock.EnvFile, "")
	emojiblock.Reload()

	all := s.AllStickers(user, 0)
	if all.GetHash() == before {
		t.Fatal("sticker list hash did not change after an item was blocked")
	}
	if all.GetPredicateName() != mtproto.Predicate_messages_allStickers {
		t.Fatalf("predicate %s", all.GetPredicateName())
	}
	setAfter, err := s.GetSet(user, shortName(ShortTwemoji), 0)
	if err != nil {
		t.Fatal(err)
	}
	if hasAlt(setAfter.GetDocuments(), grin) {
		t.Fatal("blocked grinning face is still in the sticker set")
	}
	if len(setAfter.GetDocuments()) == 0 || !hasAlt(setAfter.GetDocuments(), emojiFromCode("1f44d")) {
		t.Fatal("unrelated stickers were removed")
	}
	if len(s.CustomEmojiIDs(grin)) != 0 {
		t.Fatal("blocked custom emoji id was returned")
	}
	for _, reaction := range s.Reactions(0).GetReactions() {
		if reaction.GetReaction() == grin {
			t.Fatal("available reactions still include the blocked emoji")
		}
	}
	if s.Reactions(0).GetReactions()[0].GetReaction() != emojiFromCode("1f44d") {
		t.Fatal("default reaction was dropped")
	}
	kw := s.Keywords("en", 0)
	if kw.GetVersion() == 1 {
		t.Fatal("keyword version stayed 1 after the blocklist grew")
	}
	for _, keyword := range kw.GetKeywords() {
		for _, emo := range keyword.GetEmoticons() {
			if emo == grin {
				t.Fatalf("keyword %q still lists the blocked emoji", keyword.GetKeyword())
			}
		}
	}
}

func stickerItem(id int64, emoticon string, custom bool) *Item {
	return &Item{
		DocumentID:  id,
		AccessHash:  id,
		Emoticon:    emoticon,
		MIME:        "image/webp",
		FileName:    "x.webp",
		Width:       72,
		Height:      72,
		CustomEmoji: custom,
		Size:        4,
	}
}

func shortName(name string) *mtproto.InputStickerSet {
	return mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: name}).To_InputStickerSet()
}

func docAlt(doc *mtproto.Document) string {
	if doc == nil {
		return ""
	}
	for _, attr := range doc.GetAttributes() {
		if attr.GetAlt() != "" {
			return attr.GetAlt()
		}
	}
	return ""
}

func alts(docs []*mtproto.Document) []string {
	out := make([]string, len(docs))
	for i, doc := range docs {
		out[i] = docAlt(doc)
	}
	return out
}

func hasAlt(docs []*mtproto.Document, alt string) bool {
	for _, doc := range docs {
		if docAlt(doc) == alt {
			return true
		}
	}
	return false
}
