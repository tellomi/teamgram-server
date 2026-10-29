package catalog

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBuiltinFormats(t *testing.T) {
	webp, ok := readAsset("assets/twemoji/1f600.webp")
	if !ok || !bytes.HasPrefix(webp, []byte("RIFF")) || !bytes.Contains(webp[:16], []byte("WEBP")) {
		t.Fatal("twemoji webp is missing or not a webp")
	}
	tgs, ok := readAsset("assets/noto/1f600.tgs")
	if !ok || tgs[0] != 0x1f || tgs[1] != 0x8b {
		t.Fatal("noto tgs is missing or not gzip")
	}
	r, err := gzip.NewReader(bytes.NewReader(tgs))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fluent, ok := readAsset("assets/fluent/1f600.webp")
	if !ok || !bytes.HasPrefix(fluent, []byte("RIFF")) {
		t.Fatal("fluent webp is missing")
	}
}

func TestCatalogAPIs(t *testing.T) {
	s := newService(builtinPacks())
	const user int64 = 7

	all := s.AllStickers(user, 0)
	if all.GetPredicateName() != mtproto.Predicate_messages_allStickers {
		t.Fatalf("all stickers predicate %s", all.GetPredicateName())
	}
	var sawTwemoji bool
	for _, set := range all.GetSets() {
		if set.GetShortName() == ShortTwemoji {
			sawTwemoji = true
			if set.GetInstalledDate() == nil {
				t.Fatal("twemoji is not installed")
			}
		}
		if set.GetShortName() == ShortAnimatedEmoji {
			t.Fatal("system animated set was listed in getAllStickers")
		}
	}
	if !sawTwemoji {
		t.Fatal("twemoji was not installed by default")
	}
	again := s.AllStickers(user, all.GetHash())
	if again.GetPredicateName() != mtproto.Predicate_messages_allStickersNotModified {
		t.Fatalf("hash %d did not hit notModified (%s)", all.GetHash(), again.GetPredicateName())
	}

	emojiSets := s.EmojiStickers(user, 0)
	if len(emojiSets.GetSets()) == 0 || emojiSets.GetSets()[0].GetShortName() != ShortNoto {
		t.Fatalf("emoji stickers %#v", emojiSets.GetSets())
	}

	animated := mtproto.MakeTLInputStickerSetAnimatedEmoji(&mtproto.InputStickerSet{}).To_InputStickerSet()
	got, err := s.GetSet(user, animated, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSet().GetShortName() != ShortAnimatedEmoji || len(got.GetDocuments()) == 0 {
		t.Fatalf("animated set %#v", got.GetSet())
	}
	if got.GetDocuments()[0].GetMimeType() != "application/x-tgsticker" {
		t.Fatalf("mime %s", got.GetDocuments()[0].GetMimeType())
	}
	var custom, sticker bool
	for _, attr := range got.GetDocuments()[0].GetAttributes() {
		switch attr.GetPredicateName() {
		case mtproto.Predicate_documentAttributeCustomEmoji:
			custom = attr.GetFree()
		case mtproto.Predicate_documentAttributeSticker:
			sticker = attr.GetAlt() != ""
		}
	}
	if !custom || !sticker {
		t.Fatal("animated document is missing sticker or free custom-emoji attributes")
	}
	for _, name := range []string{
		mtproto.Predicate_inputStickerSetAnimatedEmojiAnimations,
		mtproto.Predicate_inputStickerSetEmojiGenericAnimations,
		mtproto.Predicate_inputStickerSetPremiumGifts,
		mtproto.Predicate_inputStickerSetEmojiDefaultStatuses,
		mtproto.Predicate_inputStickerSetEmojiDefaultTopicIcons,
		mtproto.Predicate_inputStickerSetEmojiChannelDefaultStatuses,
	} {
		in := &mtproto.InputStickerSet{PredicateName: name}
		if _, err = s.GetSet(user, in, 0); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	featured := s.Featured(0, false)
	if len(featured.GetSets()) == 0 {
		t.Fatal("no featured sticker sets")
	}
	found := s.SearchSets("fluent", 0, false)
	if len(found.GetSets()) != 1 || found.GetSets()[0].GetSet().GetShortName() != ShortFluent {
		t.Fatalf("search %#v", found.GetSets())
	}
	if _, err = s.Install(user, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: ShortFluent}).To_InputStickerSet(), false); err != nil {
		t.Fatal(err)
	}
	if !listed(s.AllStickers(user, 0), ShortFluent) {
		t.Fatal("install did not add Fluent")
	}
	if err = s.Uninstall(user, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: ShortTwemoji}).To_InputStickerSet()); err != nil {
		t.Fatal(err)
	}
	if listed(s.AllStickers(user, 0), ShortTwemoji) {
		t.Fatal("uninstall left Twemoji installed")
	}

	grin := emojiFromCode("1f600")
	docs := s.StickersByEmoji(grin, 0, false)
	if len(docs.GetStickers()) == 0 {
		t.Fatal("no static sticker for grinning face")
	}
	id := docs.GetStickers()[0].GetId()
	if err = s.SaveRecent(user, id, false, false); err != nil {
		t.Fatal(err)
	}
	recent := s.Recent(user, 0, false)
	if len(recent.GetStickers()) != 1 || recent.GetStickers()[0].GetId() != id {
		t.Fatalf("recent %#v", recent.GetStickers())
	}
	if err = s.Fave(user, id, false); err != nil {
		t.Fatal(err)
	}
	if len(s.Faved(user, 0).GetStickers()) != 1 {
		t.Fatal("fave did not stick")
	}
	if err = s.Fave(user, id, true); err != nil {
		t.Fatal(err)
	}
	if len(s.Faved(user, 0).GetStickers()) != 0 {
		t.Fatal("unfave left a sticker")
	}

	kw := s.Keywords("en", 0)
	if kw.GetVersion() != 1 || len(kw.GetKeywords()) == 0 {
		t.Fatalf("keywords %#v", kw)
	}
	if s.Keywords("en", 1).GetKeywords() != nil && len(s.Keywords("en", 1).GetKeywords()) != 0 {
		t.Fatal("from_version did not short-circuit")
	}
	reactions := s.Reactions(0)
	if len(reactions.GetReactions()) == 0 || reactions.GetReactions()[0].GetStaticIcon() == nil {
		t.Fatal("reactions are empty")
	}
	same := s.Reactions(reactions.GetHash())
	if same.GetPredicateName() != mtproto.Predicate_messages_availableReactionsNotModified {
		t.Fatalf("reactions hash %s", same.GetPredicateName())
	}
	customDocs := s.Documents(s.CustomEmojiIDs(grin))
	if len(customDocs.Datas) == 0 || customDocs.Datas[0].GetId() == 0 {
		t.Fatal("custom emoji documents missing")
	}
	loc, err := s.ThumbLocation(mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: ShortTwemoji}).To_InputStickerSet())
	if err != nil || loc.GetPredicateName() != mtproto.Predicate_inputDocumentFileLocation || !strings.HasSuffix(loc.GetPredicateName(), "Location") {
		t.Fatalf("thumb %#v %v", loc, err)
	}
}

func listed(all *mtproto.Messages_AllStickers, short string) bool {
	for _, set := range all.GetSets() {
		if set.GetShortName() == short {
			return true
		}
	}
	return false
}
