package catalog

import (
	"strconv"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestChineseKeywordSearch(t *testing.T) {
	s := newService(builtinPacks())
	if s.Keywords("en", 0).GetVersion() != 1 {
		t.Fatal("english keyword version moved")
	}
	item := &Item{
		DocumentID:   9001,
		AccessHash:   1,
		Emoticon:     "😂",
		MIME:         "image/webp",
		FileName:     "panda.webp",
		Width:        512,
		Height:       512,
		StorageClass: "stickers",
		Keywords:     []string{"熊猫头"},
		LangKeywords: map[string][]string{
			"zh-hant": {"貓熊"},
			"en":      {"panda"},
		},
	}
	second := &Item{
		DocumentID:   9002,
		AccessHash:   1,
		Emoticon:     "🤣",
		MIME:         "image/webp",
		FileName:     "panda2.webp",
		Width:        512,
		Height:       512,
		StorageClass: "stickers",
		Keywords:     []string{"熊猫头"},
	}
	s.addPack(&Pack{
		ID:         9000,
		AccessHash: 1,
		ShortName:  "PandaHead",
		Title:      "猫猫１号",
		Kind:       KindSticker,
		Featured:   true,
		Items:      []*Item{item, second},
	})

	en := s.Keywords("en", 0)
	if en.GetVersion() == 1 {
		t.Fatal("a new english keyword did not bump the keyword version")
	}
	if !keywordHas(en, "panda", "😂") || keywordHas(en, "熊猫头", "😂") {
		t.Fatalf("english keywords %#v", en.GetKeywords())
	}
	if len(s.Keywords("en", en.GetVersion()).GetKeywords()) != 0 {
		t.Fatal("english from_version did not short-circuit")
	}
	langs := s.Languages([]string{"zh-CN", "zh-hans", "zh", "en", "fr"})
	var got []string
	for _, lang := range langs.GetDatas() {
		got = append(got, lang.GetLangCode())
	}
	if strings.Join(got, ",") != "zh-CN,zh-hans,zh,en" {
		t.Fatalf("languages %v", got)
	}
	if len(s.Languages(nil).GetDatas()) < 2 {
		t.Fatal("empty language request did not list chinese")
	}

	for _, code := range []string{"zh-hans", "zh-CN", "zh"} {
		kw := s.Keywords(code, 0)
		if kw.GetLangCode() != code {
			t.Fatalf("keywords lang %q echoed %q", code, kw.GetLangCode())
		}
		if kw.GetVersion() <= 1 || !keywordHas(kw, "熊猫头", "😂") {
			t.Fatalf("%s keywords %#v", code, kw.GetKeywords())
		}
		if keywordHas(kw, "grinning", "😀") || keywordHas(kw, "panda", "😂") || keywordHas(kw, "貓熊", "😂") {
			t.Fatalf("%s keyword list mixed languages: %#v", code, kw.GetKeywords())
		}
		if s.Keywords(code, kw.GetVersion()).GetKeywords() != nil && len(s.Keywords(code, kw.GetVersion()).GetKeywords()) != 0 {
			t.Fatal("chinese from_version did not short-circuit")
		}
	}
	hant := s.Keywords("zh-hant", 0)
	if hant.GetLangCode() != "zh-hant" || !keywordHas(hant, "貓熊", "😂") || keywordHas(hant, "熊猫头", "😂") {
		t.Fatalf("traditional keywords %#v", hant.GetKeywords())
	}
	sets := s.SearchSets("猫猫1", 0, false)
	if len(sets.GetSets()) != 1 || sets.GetSets()[0].GetSet().GetTitle() != "猫猫１号" {
		t.Fatalf("width-folded title search %#v", sets.GetSets())
	}
	byWord := s.SearchSets("熊猫", 0, false)
	if len(byWord.GetSets()) != 1 || byWord.GetSets()[0].GetSet().GetShortName() != "PandaHead" {
		t.Fatalf("keyword pack search %#v", byWord.GetSets())
	}
	if len(s.SearchSets("ｆＬｕＥｎＴ", 0, false).GetSets()) != 0 {
		t.Fatal("foreign seed pack is still in pack search")
	}
	if again := s.SearchSets("熊猫", byWord.GetHash(), false); again.GetPredicateName() != mtproto.Predicate_messages_foundStickerSetsNotModified {
		t.Fatalf("search sets hash %s", again.GetPredicateName())
	}

	found := s.SearchStickers("熊猫", "", []string{"zh-CN"}, false, 0, 50, 0)
	if len(found.GetStickers()) != 2 || !hasDoc(found.GetStickers(), 9001) || !hasDoc(found.GetStickers(), 9002) {
		t.Fatalf("chinese keyword search %#v", found.GetStickers())
	}
	byEmoji := s.SearchStickers("😂", "", []string{"en"}, false, 0, 50, 0)
	if !hasDoc(byEmoji.GetStickers(), 9001) {
		t.Fatal("emoji query missed the chinese sticker")
	}
	// Android sends the local keyword hit as a concatenation in emoticon.
	byList := s.SearchStickers("笑", "😂🤣", []string{"zh-Hans"}, false, 0, 50, 0)
	if !hasDoc(byList.GetStickers(), 9001) {
		t.Fatal("emoticon list missed the chinese sticker")
	}
	paged := s.SearchStickers("熊猫头", "", []string{"zh"}, false, 0, 1, 0)
	if len(paged.GetStickers()) != 1 || paged.GetNextOffset() == nil || paged.GetNextOffset().GetValue() != 1 {
		t.Fatalf("page next=%v len=%d", paged.GetNextOffset(), len(paged.GetStickers()))
	}
	page2 := s.SearchStickers("熊猫头", "", []string{"zh"}, false, paged.GetNextOffset().GetValue(), 1, 0)
	if len(page2.GetStickers()) != 1 || page2.GetStickers()[0].GetId() == paged.GetStickers()[0].GetId() {
		t.Fatalf("second page %#v", page2.GetStickers())
	}

	bare := emojiFromCode("2764")
	s.addPack(&Pack{
		ID: 9010, AccessHash: 1, ShortName: "Hearts", Title: "红心",
		Kind: KindSticker, InstalledDefault: true,
		Items: []*Item{{
			DocumentID: 9011, AccessHash: 1, Emoticon: bare,
			MIME: "image/webp", FileName: "heart.webp", Width: 512, Height: 512,
		}},
	})
	if len(s.StickersByEmoji(bare+"\uFE0F", 0, false).GetStickers()) == 0 {
		t.Fatal("heart with variation selector did not match")
	}
}

func TestImportedPackIsDefaultInstalledUntilRemoved(t *testing.T) {
	s := newService(builtinPacks())
	const smile = "\U0001F642"
	item := &Item{
		DocumentID: 9101, AccessHash: 1, Emoticon: smile,
		MIME: "image/webp", FileName: "happy.webp", Width: 512, Height: 512,
		Keywords: []string{"开心"},
	}
	s.addPack(&Pack{
		ID: 9100, AccessHash: 1, ShortName: "HappyBQB", Title: "开心包",
		Kind: KindSticker, Official: true, Featured: true, InstalledDefault: true,
		Items: []*Item{item},
	})
	emojiItem := &Item{
		DocumentID: 9102, AccessHash: 1, Emoticon: smile,
		MIME: "image/webp", FileName: "happy-emoji.webp", Width: 512, Height: 512,
		CustomEmoji: true, Keywords: []string{"开心"},
	}
	s.addPack(&Pack{
		ID: 9103, AccessHash: 1, ShortName: "HappyEmoji", Title: "开心表情",
		Kind: KindCustomEmoji, Official: true, Emojis: true, Featured: true, InstalledDefault: true,
		Items: []*Item{emojiItem},
	})

	for _, user := range []int64{1, 2} {
		if !listed(s.AllStickers(user, 0), "HappyBQB") {
			t.Fatalf("user %d is missing the default sticker pack", user)
		}
		if !listed(s.EmojiStickers(user, 0), "HappyEmoji") {
			t.Fatalf("user %d is missing the default custom-emoji pack", user)
		}
	}
	if len(s.Featured(1, 0, false).GetSets()) == 0 || len(s.Featured(1, 0, true).GetSets()) == 0 {
		t.Fatal("default packs are not featured")
	}
	sets := s.SearchSets("开心", 0, false)
	if len(sets.GetSets()) != 1 || sets.GetSets()[0].GetSet().GetShortName() != "HappyBQB" {
		t.Fatalf("pack search 开心 %#v", sets.GetSets())
	}
	byEmoji := s.SearchSets(smile, 0, false)
	if len(byEmoji.GetSets()) != 1 || byEmoji.GetSets()[0].GetSet().GetShortName() != "HappyBQB" {
		t.Fatalf("pack search emoji %#v", byEmoji.GetSets())
	}
	docs := s.SearchStickers("开心", "", []string{"zh-hans"}, false, 0, 20, 0)
	if len(docs.GetStickers()) != 1 || docs.GetStickers()[0].GetId() != 9101 {
		t.Fatalf("sticker search %#v", docs.GetStickers())
	}
	kw := s.Keywords("zh-CN", 0)
	if kw.GetLangCode() != "zh-CN" || !keywordHas(kw, "开心", smile) {
		t.Fatalf("keywords %#v", kw.GetKeywords())
	}
	in := mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: "HappyBQB"}).To_InputStickerSet()
	if err := s.Uninstall(1, in); err != nil {
		t.Fatal(err)
	}
	if listed(s.AllStickers(1, 0), "HappyBQB") {
		t.Fatal("uninstall did not remove the pack for that user")
	}
	if !listed(s.AllStickers(2, 0), "HappyBQB") {
		t.Fatal("uninstall removed the pack for another user")
	}
}

func TestFeaturedAndInstalledScale(t *testing.T) {
	s := newService(builtinPacks())
	const extra = 120
	for i := 0; i < extra; i++ {
		s.addPack(&Pack{
			ID:               int64(10000 + i),
			AccessHash:       1,
			ShortName:        "Bulk" + strconv.Itoa(i),
			Title:            "批量 " + strconv.Itoa(i),
			Kind:             KindSticker,
			Featured:         true,
			InstalledDefault: true,
			Items: []*Item{{
				DocumentID:   int64(20000 + i),
				AccessHash:   1,
				Emoticon:     "😀",
				MIME:         "image/webp",
				FileName:     "a.webp",
				Width:        512,
				Height:       512,
				StorageClass: "stickers",
			}},
		})
	}
	featured := s.Featured(1, 0, false)
	if featured.GetCount() < extra || len(featured.GetSets()) != featuredPageSize {
		t.Fatalf("featured count=%d sets=%d", featured.GetCount(), len(featured.GetSets()))
	}
	if again := s.Featured(1, featured.GetHash(), false); again.GetPredicateName() != mtproto.Predicate_messages_featuredStickersNotModified {
		t.Fatalf("featured hash %s", again.GetPredicateName())
	}
	rest := s.OldFeatured(1, 0, featuredPageSize, 50)
	want := int(featured.GetCount()) - featuredPageSize
	if len(rest.GetSets()) != want || rest.GetCount() != featured.GetCount() {
		t.Fatalf("old featured sets=%d count=%d want sets=%d", len(rest.GetSets()), rest.GetCount(), want)
	}
	if len(s.OldFeatured(1, 0, featured.GetCount(), 10).GetSets()) != 0 {
		t.Fatal("old featured past the end returned sets")
	}

	all := s.AllStickers(3, 0)
	if len(all.GetSets()) < extra {
		t.Fatalf("installed sets %d", len(all.GetSets()))
	}
	if again := s.AllStickers(3, all.GetHash()); again.GetPredicateName() != mtproto.Predicate_messages_allStickersNotModified {
		t.Fatalf("all stickers hash %s", again.GetPredicateName())
	}
}

func keywordHas(diff *mtproto.EmojiKeywordsDifference, word, emo string) bool {
	for _, kw := range diff.GetKeywords() {
		if kw.GetKeyword() != word {
			continue
		}
		for _, got := range kw.GetEmoticons() {
			if got == emo {
				return true
			}
		}
	}
	return false
}
