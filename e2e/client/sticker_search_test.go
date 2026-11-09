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
	if !ok || len(sets.GetSets()) == 0 || sets.GetSets()[0].GetSet().GetShortName() != "FluentEmoji" {
		t.Fatalf("fullwidth set search = %T %#v", found, found)
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
	docs, ok := stickers.(*mtproto.TLMessagesFoundStickers)
	if !ok || len(docs.GetStickers()) == 0 {
		t.Fatalf("keyword search = %T %#v", stickers, stickers)
	}

	all := invokeOK(t, a, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
	})
	installed, ok := all.(*mtproto.TLMessagesAllStickers)
	if !ok || len(installed.GetSets()) == 0 || installed.GetHash() == 0 {
		t.Fatalf("all stickers = %T %#v", all, all)
	}
	again := invokeOK(t, a, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
		Hash:        installed.GetHash(),
	})
	if _, ok = again.(*mtproto.TLMessagesAllStickersNotModified); !ok {
		t.Fatalf("all stickers hash = %T", again)
	}

	featured := invokeOK(t, a, &mtproto.TLMessagesGetFeaturedStickers{
		Constructor: mtproto.CRC32_messages_getFeaturedStickers,
	})
	page, ok := featured.(*mtproto.TLMessagesFeaturedStickers)
	if !ok || page.GetCount() < 1 || len(page.GetSets()) == 0 || int(page.GetCount()) < len(page.GetSets()) {
		t.Fatalf("featured = %T %+v", featured, featured)
	}
	cached := invokeOK(t, a, &mtproto.TLMessagesGetFeaturedStickers{
		Constructor: mtproto.CRC32_messages_getFeaturedStickers,
		Hash:        page.GetHash(),
	})
	if _, ok = cached.(*mtproto.TLMessagesFeaturedStickersNotModified); !ok {
		t.Fatalf("featured hash = %T", cached)
	}
	old := invokeOK(t, a, &mtproto.TLMessagesGetOldFeaturedStickers{
		Constructor: mtproto.CRC32_messages_getOldFeaturedStickers,
		Offset:      0,
		Limit:       10,
	})
	oldPage, ok := old.(*mtproto.TLMessagesFeaturedStickers)
	if !ok || oldPage.GetCount() != page.GetCount() || len(oldPage.GetSets()) == 0 {
		t.Fatalf("old featured = %T %+v", old, old)
	}
}
