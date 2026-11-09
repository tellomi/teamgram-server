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
	if len(s.SearchSets("熊猫", 0, false).GetSets()) != 0 {
		t.Fatal("title search matched a keyword")
	}
	fluent := s.SearchSets("ｆＬｕＥｎＴ", 0, false)
	if len(fluent.GetSets()) != 1 || fluent.GetSets()[0].GetSet().GetShortName() != ShortFluent {
		t.Fatalf("fullwidth title search %#v", fluent.GetSets())
	}
	if again := s.SearchSets("ｆＬｕＥｎＴ", fluent.GetHash(), false); again.GetPredicateName() != mtproto.Predicate_messages_foundStickerSetsNotModified {
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

	heart := emojiFromCode("2764") + "\uFE0F"
	if len(s.StickersByEmoji(heart, 0, false).GetStickers()) == 0 {
		t.Fatal("heart with variation selector did not match")
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
