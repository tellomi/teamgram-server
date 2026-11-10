package catalog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestTelegramHashMatchesClientSteps(t *testing.T) {
	if telegramHash(nil) != 0 || telegramHash([]int64{}) != 0 {
		t.Fatal("empty list must hash to 0")
	}
	// acc starts at 0, so the first value is added unchanged.
	if telegramHash([]int64{1}) != 1 {
		t.Fatal(telegramHash([]int64{1}))
	}
	// Sign-extended negative int32. Java passes set.hash as a long.
	neg := int64(int32(-1))
	var acc uint64
	acc ^= acc >> 21
	acc ^= acc << 35
	acc ^= acc >> 4
	acc += uint64(neg)
	if telegramHash([]int64{neg}) != int64(acc) {
		t.Fatalf("sign-extended hash %d want %d", telegramHash([]int64{neg}), int64(acc))
	}
	ids := []int64{1, 2, 3}
	acc = 0
	for _, id := range ids {
		acc ^= acc >> 21
		acc ^= acc << 35
		acc ^= acc >> 4
		acc += uint64(id)
	}
	if telegramHash(ids) != int64(acc) {
		t.Fatalf("got %d want %d", telegramHash(ids), int64(acc))
	}
}

func TestInstalledAndFeaturedHashesMatchClientInputs(t *testing.T) {
	s := newService(nil)
	a := hotPack(10, "AlphaPack", "Alpha", false, false)
	b := hotPack(20, "BetaPack", "Beta", false, false)
	b.Featured = true
	s.addPack(a)
	s.addPack(b)

	all := s.AllStickers(1, 0)
	var setHashes []int64
	for _, set := range all.GetSets() {
		if set.GetArchived() {
			continue
		}
		setHashes = append(setHashes, int64(set.GetHash()))
	}
	if all.GetHash() != telegramHash(setHashes) {
		t.Fatalf("all stickers %d client %d", all.GetHash(), telegramHash(setHashes))
	}
	if again := s.AllStickers(1, all.GetHash()); again.GetPredicateName() != mtproto.Predicate_messages_allStickersNotModified {
		t.Fatal(again.GetPredicateName())
	}
	if zero := s.AllStickers(1, 0); zero.GetPredicateName() == mtproto.Predicate_messages_allStickersNotModified {
		t.Fatal("hash 0 must return the list")
	}

	featured := s.Featured(1, 0, false)
	unread := map[int64]bool{}
	for _, id := range featured.GetUnread() {
		unread[id] = true
	}
	var feat []int64
	for _, covered := range featured.GetSets() {
		id := covered.GetSet().GetId()
		feat = append(feat, id)
		if unread[id] {
			feat = append(feat, 1)
		}
	}
	if featured.GetHash() != telegramHash(feat) {
		t.Fatalf("featured %d client %d", featured.GetHash(), telegramHash(feat))
	}
	if again := s.Featured(1, featured.GetHash(), false); again.GetPredicateName() != mtproto.Predicate_messages_featuredStickersNotModified {
		t.Fatal(again.GetPredicateName())
	}
}

func TestDocumentListHashStopsAt200(t *testing.T) {
	ids := make([]int64, 201)
	docs := make([]*mtproto.Document, 201)
	for i := range ids {
		ids[i] = int64(i + 1)
		docs[i] = mtproto.MakeTLDocument(&mtproto.Document{Id: ids[i]}).To_Document()
	}
	res := stickersResult(docs, ids, 0)
	if res.GetHash() != telegramHash(ids[:clientDocumentHashLimit]) {
		t.Fatalf("hash %d", res.GetHash())
	}
	if len(res.GetStickers()) != 201 {
		t.Fatalf("len %d", len(res.GetStickers()))
	}
	if again := stickersResult(docs, ids, res.GetHash()); again.GetPredicateName() != mtproto.Predicate_messages_stickersNotModified {
		t.Fatal(again.GetPredicateName())
	}
}
