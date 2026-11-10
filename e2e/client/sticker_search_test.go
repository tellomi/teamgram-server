package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestStickerSearchAndKeywordLanguages(t *testing.T) {
	a, _, _, _ := sharedPair(t)

	en := invokeOK(t, a, &mtproto.TLMessagesGetEmojiKeywords{
		Constructor: mtproto.CRC32_messages_getEmojiKeywords,
		LangCode:    "en",
	})
	enKW, ok := en.(*mtproto.TLEmojiKeywordsDifference)
	if !ok || enKW.GetLangCode() != "en" || enKW.GetVersion() < 1 || len(enKW.GetKeywords()) == 0 {
		t.Fatalf("en keywords = %T %+v", en, en)
	}

	for _, code := range []string{"zh-hans", "zh-CN", "zh"} {
		res := invokeOK(t, a, &mtproto.TLMessagesGetEmojiKeywords{
			Constructor: mtproto.CRC32_messages_getEmojiKeywords,
			LangCode:    code,
		})
		kw, ok := res.(*mtproto.TLEmojiKeywordsDifference)
		if !ok || kw.GetLangCode() != code {
			t.Fatalf("keywords %s = %T %+v", code, res, res)
		}
	}

	langs := invokeOK(t, a, &mtproto.TLMessagesGetEmojiKeywordsLanguages{
		Constructor: mtproto.CRC32_messages_getEmojiKeywordsLanguages,
		LangCodes:   []string{"zh-CN", "en", "fr"},
	})
	vec, ok := langs.(*tlVector)
	if !ok || vec.Count < 1 {
		t.Fatalf("keyword languages = %T %+v", langs, langs)
	}
	onlyZH := invokeOK(t, a, &mtproto.TLMessagesGetEmojiKeywordsLanguages{
		Constructor: mtproto.CRC32_messages_getEmojiKeywordsLanguages,
		LangCodes:   []string{"zh-CN"},
	})
	zhVec, ok := onlyZH.(*tlVector)
	// The seed has no simplified-Chinese keywords, so zh-CN is not offered.
	// An imported Chinese pack makes this count 1. Either result decodes.
	if !ok || zhVec.Count > 1 {
		t.Fatalf("zh-CN languages = %T %+v", onlyZH, onlyZH)
	}

	found := invokeOK(t, a, &mtproto.TLMessagesSearchStickerSets{
		Constructor: mtproto.CRC32_messages_searchStickerSets,
		Q:           "ｆＬｕＥｎＴ",
	})
	sets, ok := found.(*mtproto.TLMessagesFoundStickerSets)
	if !ok {
		t.Fatalf("set search = %T", found)
	}
	for _, set := range sets.GetSets() {
		switch set.GetSet().GetShortName() {
		case "Twemoji", "FluentEmoji", "NotoEmoji":
			t.Fatalf("foreign seed still searchable: %s", set.GetSet().GetShortName())
		}
	}
	same := invokeOK(t, a, &mtproto.TLMessagesSearchStickerSets{
		Constructor: mtproto.CRC32_messages_searchStickerSets,
		Q:           "ｆＬｕＥｎＴ",
		Hash:        sets.GetHash(),
	})
	if _, ok = same.(*mtproto.TLMessagesFoundStickerSetsNotModified); !ok {
		t.Fatalf("set search hash = %T", same)
	}

	stickers := invokeOK(t, a, &mtproto.TLMessagesSearchStickers{
		Constructor: mtproto.CRC32_messages_searchStickers,
		Q:           "fire",
		LangCode:    []string{"en"},
		Limit:       20,
	})
	if _, ok = stickers.(*mtproto.TLMessagesFoundStickers); !ok {
		t.Fatalf("keyword search = %T", stickers)
	}

	all := invokeOK(t, a, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
	})
	installed, ok := all.(*mtproto.TLMessagesAllStickers)
	if !ok {
		t.Fatalf("all stickers = %T %+v", all, all)
	}
	for _, set := range installed.GetSets() {
		switch set.GetShortName() {
		case "Twemoji", "FluentEmoji", "NotoEmoji":
			t.Fatalf("foreign seed still installed: %s", set.GetShortName())
		}
	}
	again := invokeOK(t, a, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
		Hash:        installed.GetHash(),
	})
	// An empty install list hashes to 0. Clients send 0 to force a refresh,
	// so that reply is the list. A non-zero hash is the client hash.
	if installed.GetHash() == 0 {
		if len(installed.GetSets()) != 0 {
			t.Fatalf("non-empty all stickers hashed to 0")
		}
		if _, ok = again.(*mtproto.TLMessagesAllStickers); !ok {
			t.Fatalf("hash 0 all stickers = %T", again)
		}
	} else if _, ok = again.(*mtproto.TLMessagesAllStickersNotModified); !ok {
		t.Fatalf("all stickers hash = %T", again)
	}

	featured := invokeOK(t, a, &mtproto.TLMessagesGetFeaturedStickers{
		Constructor: mtproto.CRC32_messages_getFeaturedStickers,
	})
	page, ok := featured.(*mtproto.TLMessagesFeaturedStickers)
	if !ok || int(page.GetCount()) < len(page.GetSets()) {
		t.Fatalf("featured = %T %+v", featured, featured)
	}
	for _, set := range page.GetSets() {
		switch set.GetSet().GetShortName() {
		case "Twemoji", "FluentEmoji", "NotoEmoji":
			t.Fatalf("foreign seed still featured: %s", set.GetSet().GetShortName())
		}
	}
	cached := invokeOK(t, a, &mtproto.TLMessagesGetFeaturedStickers{
		Constructor: mtproto.CRC32_messages_getFeaturedStickers,
		Hash:        page.GetHash(),
	})
	if page.GetHash() == 0 {
		if len(page.GetSets()) != 0 {
			t.Fatal("non-empty featured list hashed to 0")
		}
		if _, ok = cached.(*mtproto.TLMessagesFeaturedStickers); !ok {
			t.Fatalf("hash 0 featured = %T", cached)
		}
	} else if _, ok = cached.(*mtproto.TLMessagesFeaturedStickersNotModified); !ok {
		t.Fatalf("featured hash = %T", cached)
	}
	old := invokeOK(t, a, &mtproto.TLMessagesGetOldFeaturedStickers{
		Constructor: mtproto.CRC32_messages_getOldFeaturedStickers,
		Offset:      0,
		Limit:       10,
	})
	oldPage, ok := old.(*mtproto.TLMessagesFeaturedStickers)
	if !ok || oldPage.GetCount() != page.GetCount() {
		t.Fatalf("old featured = %T %+v", old, old)
	}
}
