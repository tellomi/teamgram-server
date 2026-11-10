package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// clientListHash is Android MediaDataController.calcHash and iOS
// combineInt64Hash. It is independent of the server implementation: the
// test fails when the server hash is not the one a real client sends back.
func clientListHash(ids []int64) int64 {
	if len(ids) == 0 {
		return 0
	}
	var acc uint64
	for _, id := range ids {
		acc ^= acc >> 21
		acc ^= acc << 35
		acc ^= acc >> 4
		acc += uint64(id)
	}
	return int64(acc)
}

func clientDocumentHash(docs []*mtproto.Document) int64 {
	var ids []int64
	for _, doc := range docs {
		if doc == nil || doc.GetId() == 0 {
			continue
		}
		ids = append(ids, doc.GetId())
		if len(ids) == 200 {
			break
		}
	}
	return clientListHash(ids)
}

func TestClientListHashNotModified(t *testing.T) {
	cfg := requireE2E(t)
	c, self := loginNamed(t, cfg, ProfileIOS, "hashcache", "Hash")

	// The compose stack ships no installed pack, so getAllStickers is empty
	// and its hash is 0. Create one pack, then hash it the way a client does.
	doc := uploadStickerDocument(t, c, "image/webp", "sticker.webp", tinyStickerWEBP)
	created := createStickerSet(t, c, self, "", []*mtproto.InputStickerSetItem{stickerItem(doc, "😀")})
	if created.GetSet().GetHash() == 0 {
		t.Fatal("created set has no hash")
	}

	all := invokeOK(t, c, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
	})
	installed, ok := all.(*mtproto.TLMessagesAllStickers)
	if !ok || len(installed.GetSets()) == 0 {
		t.Fatalf("getAllStickers %T %+v", all, all)
	}
	var setHashes []int64
	for _, set := range installed.GetSets() {
		if set.GetArchived() {
			continue
		}
		setHashes = append(setHashes, int64(set.GetHash()))
	}
	if installed.GetHash() != clientListHash(setHashes) || installed.GetHash() == 0 {
		t.Fatalf("getAllStickers server %d client %d (%d sets)", installed.GetHash(), clientListHash(setHashes), len(setHashes))
	}
	again := invokeOK(t, c, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
		Hash:        clientListHash(setHashes),
	})
	if _, ok = again.(*mtproto.TLMessagesAllStickersNotModified); !ok {
		t.Fatalf("getAllStickers NotModified %T", again)
	}
	miss := int64(1)
	if installed.GetHash() == miss {
		miss = 2
	}
	wrong := invokeOK(t, c, &mtproto.TLMessagesGetAllStickers{
		Constructor: mtproto.CRC32_messages_getAllStickers,
		Hash:        miss,
	})
	if _, ok = wrong.(*mtproto.TLMessagesAllStickers); !ok {
		t.Fatalf("mismatched hash %T", wrong)
	}

	checkAll := func(name string, req func(int64) mtproto.TLObject) {
		t.Helper()
		first := invokeOK(t, c, req(0))
		page, ok := first.(*mtproto.TLMessagesAllStickers)
		if !ok {
			t.Fatalf("%s %T", name, first)
		}
		var ids []int64
		for _, set := range page.GetSets() {
			if set.GetArchived() {
				continue
			}
			ids = append(ids, int64(set.GetHash()))
		}
		if page.GetHash() != clientListHash(ids) {
			t.Fatalf("%s server %d client %d", name, page.GetHash(), clientListHash(ids))
		}
		if page.GetHash() == 0 {
			second := invokeOK(t, c, req(0))
			if _, ok = second.(*mtproto.TLMessagesAllStickersNotModified); ok {
				t.Fatalf("%s hash 0 returned NotModified", name)
			}
			return
		}
		second := invokeOK(t, c, req(clientListHash(ids)))
		if _, ok = second.(*mtproto.TLMessagesAllStickersNotModified); !ok {
			t.Fatalf("%s NotModified %T", name, second)
		}
	}
	checkAll("getMaskStickers", func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetMaskStickers{Constructor: mtproto.CRC32_messages_getMaskStickers, Hash: hash}
	})
	checkAll("getEmojiStickers", func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetEmojiStickers{Constructor: mtproto.CRC32_messages_getEmojiStickers, Hash: hash}
	})

	checkFeatured := func(name string, req func(int64) mtproto.TLObject) {
		t.Helper()
		first := invokeOK(t, c, req(0))
		page, ok := first.(*mtproto.TLMessagesFeaturedStickers)
		if !ok {
			t.Fatalf("%s %T", name, first)
		}
		unread := map[int64]bool{}
		for _, id := range page.GetUnread() {
			unread[id] = true
		}
		var ids []int64
		for _, covered := range page.GetSets() {
			set := covered.GetSet()
			if set == nil || set.GetArchived() {
				continue
			}
			ids = append(ids, set.GetId())
			if unread[set.GetId()] {
				ids = append(ids, 1)
			}
		}
		if page.GetHash() != clientListHash(ids) {
			t.Fatalf("%s server %d client %d", name, page.GetHash(), clientListHash(ids))
		}
		if page.GetHash() == 0 {
			second := invokeOK(t, c, req(0))
			if _, ok = second.(*mtproto.TLMessagesFeaturedStickersNotModified); ok {
				t.Fatalf("%s hash 0 returned NotModified", name)
			}
			return
		}
		second := invokeOK(t, c, req(clientListHash(ids)))
		if _, ok = second.(*mtproto.TLMessagesFeaturedStickersNotModified); !ok {
			t.Fatalf("%s NotModified %T", name, second)
		}
	}
	checkFeatured("getFeaturedStickers", func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetFeaturedStickers{Constructor: mtproto.CRC32_messages_getFeaturedStickers, Hash: hash}
	})
	checkFeatured("getFeaturedEmojiStickers", func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetFeaturedEmojiStickers{Constructor: mtproto.CRC32_messages_getFeaturedEmojiStickers, Hash: hash}
	})

	byEmoji := invokeOK(t, c, &mtproto.TLMessagesGetStickers{
		Constructor: mtproto.CRC32_messages_getStickers,
		Emoticon:    "😀",
	})
	emojiPage, ok := byEmoji.(*mtproto.TLMessagesStickers)
	if !ok {
		t.Fatalf("getStickers %T", byEmoji)
	}
	if emojiPage.GetHash() != clientDocumentHash(emojiPage.GetStickers()) {
		t.Fatalf("getStickers server %d client %d", emojiPage.GetHash(), clientDocumentHash(emojiPage.GetStickers()))
	}
	if emojiPage.GetHash() != 0 {
		cached := invokeOK(t, c, &mtproto.TLMessagesGetStickers{
			Constructor: mtproto.CRC32_messages_getStickers,
			Emoticon:    "😀",
			Hash:        clientDocumentHash(emojiPage.GetStickers()),
		})
		if _, ok = cached.(*mtproto.TLMessagesStickersNotModified); !ok {
			t.Fatalf("getStickers NotModified %T", cached)
		}
	}

	if len(installed.GetSets()) > 0 {
		full := invokeOK(t, c, &mtproto.TLMessagesGetStickerSet{
			Constructor: mtproto.CRC32_messages_getStickerSet_c8a0ec74,
			Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
				Id:         installed.GetSets()[0].GetId(),
				AccessHash: installed.GetSets()[0].GetAccessHash(),
			}).To_InputStickerSet(),
		})
		set, ok := full.(*mtproto.TLMessagesStickerSet)
		if !ok || len(set.GetDocuments()) == 0 {
			t.Fatalf("sticker set documents %T", full)
		}
		doc := set.GetDocuments()[0]
		in := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:            doc.GetId(),
			AccessHash:    doc.GetAccessHash(),
			FileReference: doc.GetFileReference(),
		}).To_InputDocument()
		invokeOK(t, c, &mtproto.TLMessagesSaveRecentSticker{
			Constructor: mtproto.CRC32_messages_saveRecentSticker,
			Id:          in,
			Unsave:      mtproto.MakeTLBoolFalse(nil).To_Bool(),
		})
		invokeOK(t, c, &mtproto.TLMessagesFaveSticker{
			Constructor: mtproto.CRC32_messages_faveSticker,
			Id:          in,
			Unfave:      mtproto.MakeTLBoolFalse(nil).To_Bool(),
		})
	}

	checkDocs := func(name string, docs []*mtproto.Document, hash int64, again func(int64) mtproto.TLObject, notModified func(mtproto.TLObject) bool) {
		t.Helper()
		want := clientDocumentHash(docs)
		if hash != want {
			t.Fatalf("%s server %d client %d", name, hash, want)
		}
		if hash == 0 {
			second := invokeOK(t, c, again(0))
			if notModified(second) {
				t.Fatalf("%s hash 0 returned NotModified", name)
			}
			return
		}
		second := invokeOK(t, c, again(want))
		if !notModified(second) {
			t.Fatalf("%s NotModified %T", name, second)
		}
	}
	recent := invokeOK(t, c, &mtproto.TLMessagesGetRecentStickers{Constructor: mtproto.CRC32_messages_getRecentStickers})
	recentPage, ok := recent.(*mtproto.TLMessagesRecentStickers)
	if !ok {
		t.Fatalf("getRecentStickers %T", recent)
	}
	checkDocs("getRecentStickers", recentPage.GetStickers(), recentPage.GetHash(), func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetRecentStickers{Constructor: mtproto.CRC32_messages_getRecentStickers, Hash: hash}
	}, func(obj mtproto.TLObject) bool {
		_, ok := obj.(*mtproto.TLMessagesRecentStickersNotModified)
		return ok
	})

	faved := invokeOK(t, c, &mtproto.TLMessagesGetFavedStickers{Constructor: mtproto.CRC32_messages_getFavedStickers})
	favedPage, ok := faved.(*mtproto.TLMessagesFavedStickers)
	if !ok {
		t.Fatalf("getFavedStickers %T", faved)
	}
	checkDocs("getFavedStickers", favedPage.GetStickers(), favedPage.GetHash(), func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetFavedStickers{Constructor: mtproto.CRC32_messages_getFavedStickers, Hash: hash}
	}, func(obj mtproto.TLObject) bool {
		_, ok := obj.(*mtproto.TLMessagesFavedStickersNotModified)
		return ok
	})

	gifs := invokeOK(t, c, &mtproto.TLMessagesGetSavedGifs{Constructor: mtproto.CRC32_messages_getSavedGifs})
	gifPage, ok := gifs.(*mtproto.TLMessagesSavedGifs)
	if !ok {
		t.Fatalf("getSavedGifs %T", gifs)
	}
	checkDocs("getSavedGifs", gifPage.GetGifs(), gifPage.GetHash(), func(hash int64) mtproto.TLObject {
		return &mtproto.TLMessagesGetSavedGifs{Constructor: mtproto.CRC32_messages_getSavedGifs, Hash: hash}
	}, func(obj mtproto.TLObject) bool {
		_, ok := obj.(*mtproto.TLMessagesSavedGifsNotModified)
		return ok
	})
}
