package catalog

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestAppendPackKeyChangesWhenPackIsEdited(t *testing.T) {
	s := newService(nil)
	pack := hotPack(9001, "HelloPack", "Hello", false, false)
	s.addPack(pack)
	regular := s.AllStickers(1, 0).GetHash()
	pack.Title = "Hello 2"
	renamed := s.AllStickers(1, 0).GetHash()
	if renamed == regular {
		t.Fatal("title edit left getAllStickers hash unchanged")
	}
	pack.Items = append(pack.Items, stickerItem(43, "😁", false))
	if s.AllStickers(1, 0).GetHash() == renamed {
		t.Fatal("document edit left getAllStickers hash unchanged")
	}

	masks := newService(nil)
	mask := hotPack(9002, "MaskPack", "Mask", true, false)
	masks.addPack(mask)
	maskHash := masks.MaskStickers(1, 0).GetHash()
	mask.Title = "Mask 2"
	if masks.MaskStickers(1, 0).GetHash() == maskHash {
		t.Fatal("title edit left getMaskStickers hash unchanged")
	}

	emoji := newService(nil)
	emo := hotPack(9003, "EmojiPack", "Emoji", false, true)
	emoji.addPack(emo)
	emojiHash := emoji.EmojiStickers(1, 0).GetHash()
	emo.Items[0].DocumentID = 99
	if emoji.EmojiStickers(1, 0).GetHash() == emojiHash {
		t.Fatal("document edit left getEmojiStickers hash unchanged")
	}
}

func TestSeedFingerprintSurvivesKeywordRoundTrip(t *testing.T) {
	for _, p := range builtinPacks() {
		cp := clonePack(p)
		for _, item := range cp.Items {
			item.Keywords, item.LangKeywords = decodeKeywords(encodeKeywords(item))
		}
		if packFingerprint(p) != packFingerprint(cp) {
			t.Fatalf("%s fingerprint changed across keyword round trip", p.ShortName)
		}
	}
}

func TestApplyImportedAddsAndReplaces(t *testing.T) {
	s := newService(builtinPacks())
	pack := hotPack(9001, "HelloPack", "Hello", false, false)
	diff := s.applyImported([]*Pack{pack}, true)
	if diff.added != 1 || diff.updated != 0 || !diff.kinds.regular {
		t.Fatalf("add %+v", diff)
	}
	got, err := s.GetSet(1, shortName("HelloPack"), 0)
	if err != nil || got.GetSet().GetTitle() != "Hello" {
		t.Fatalf("lookup %v %v", got.GetSet().GetTitle(), err)
	}
	before := s.AllStickers(1, 0).GetHash()

	next := clonePack(pack)
	next.Title = "Hello 2"
	diff = s.applyImported([]*Pack{next}, true)
	if diff.updated != 1 || diff.added != 0 {
		t.Fatalf("update %+v", diff)
	}
	got, err = s.GetSet(1, shortName("HelloPack"), 0)
	if err != nil || got.GetSet().GetTitle() != "Hello 2" {
		t.Fatalf("replaced title %v %v", got.GetSet().GetTitle(), err)
	}
	if s.AllStickers(1, 0).GetHash() == before {
		t.Fatal("replaced pack did not change the list hash")
	}

	again := clonePack(next)
	if diff = s.applyImported([]*Pack{again}, true); diff.any() {
		t.Fatalf("unchanged pack was rewritten: %+v", diff)
	}

	other := hotPack(9002, "OtherPack", "Other", false, true)
	diff = s.applyImported([]*Pack{other}, true)
	if diff.removed != 1 || !diff.kinds.emojis {
		t.Fatalf("remove %+v", diff)
	}
	if _, err = s.GetSet(1, shortName("HelloPack"), 0); err != mtproto.ErrStickersetInvalid {
		t.Fatalf("removed pack still served: %v", err)
	}
	if _, err = s.GetSet(1, shortName(ShortTwemoji), 0); err != nil {
		t.Fatal("builtin pack was dropped")
	}
}

func TestApplyImportedKeepsSystemPacks(t *testing.T) {
	s := newService(nil)
	sys := &Pack{
		ID: 77, AccessHash: 1, ShortName: "SysPack", Title: "System", Kind: KindAnimatedEmoji,
		Items: []*Item{stickerItem(1, "😀", false)},
	}
	s.applyImported([]*Pack{sys}, true)
	s.applyImported([]*Pack{hotPack(78, "KeepPack", "Keep", false, false)}, true)
	if _, err := s.GetSet(1, shortName("SysPack"), 0); err != nil {
		t.Fatal(err)
	}
}

func TestApplyImportedSafeWithReaders(t *testing.T) {
	s := newService(builtinPacks())
	pack := hotPack(9001, "HelloPack", "Hello", false, false)
	s.applyImported([]*Pack{pack}, false)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		in := shortName(ShortTwemoji)
		hot := shortName("HelloPack")
		for ctx.Err() == nil {
			if _, err := s.GetSet(1, in, 0); err != nil {
				t.Errorf("twemoji: %v", err)
				return
			}
			if _, err := s.GetSet(1, hot, 0); err != nil {
				t.Errorf("hot: %v", err)
				return
			}
			_ = s.AllStickers(1, 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			next := clonePack(pack)
			next.Title = fmt.Sprintf("Hot %d", i)
			s.applyImported([]*Pack{next}, false)
		}
		cancel()
	}()
	wg.Wait()
}

func TestCatalogBroadcastReachesRecentUsers(t *testing.T) {
	s := newService(nil)
	rec := &recordPush{}
	s.SetUserPush(rec)
	s.recentUserFn = func(context.Context) ([]int64, error) {
		return []int64{3, 4}, nil
	}
	s.noteCatalogChange(kindFlags{regular: true, emojis: true})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rec.count() >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rec.count() != 4 {
		t.Fatalf("pushes %d", rec.count())
	}
	var regular, emoji int
	for _, up := range rec.updates() {
		if len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateStickerSets {
			t.Fatalf("update %#v", up.GetUpdates())
		}
		switch {
		case up.GetUpdates()[0].GetEmojis():
			emoji++
		case !up.GetUpdates()[0].GetMasks():
			regular++
		}
	}
	if regular != 2 || emoji != 2 {
		t.Fatalf("regular %d emoji %d", regular, emoji)
	}
}

func TestStickerUpdateConstructors(t *testing.T) {
	set := hotPack(1, "HelloPack", "Hello", false, false).full(true, false)
	install := StickerInstallUpdate(set)
	up := install.GetUpdates()[0]
	if up.GetPredicateName() != mtproto.Predicate_updateNewStickerSet || up.GetStickerset_MESSAGES_STICKERSET().GetSet().GetShortName() != "HelloPack" {
		t.Fatalf("install %#v", up)
	}
	buf := mtproto.NewEncodeBuf(256)
	tl := mtproto.MakeTLUpdateNewStickerSet(up)
	if err := tl.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(buf.GetBuf()[:4]) != 0x688a30aa {
		t.Fatalf("constructor %x", buf.GetBuf()[:4])
	}

	changed := StickerSetsChanged(false, true)
	flags := mtproto.MakeTLUpdateStickerSets(changed.GetUpdates()[0])
	buf = mtproto.NewEncodeBuf(32)
	if err := flags.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if binary.LittleEndian.Uint32(raw[:4]) != 0x31c24808 || binary.LittleEndian.Uint32(raw[4:8]) != 2 {
		t.Fatalf("updateStickerSets %x", raw)
	}
	order := StickerOrderUpdate(true, false, []int64{9, 8})
	if order.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateStickerSetsOrder || !order.GetUpdates()[0].GetMasks() {
		t.Fatal(order.GetUpdates()[0])
	}
	if got := order.GetUpdates()[0].GetOrder_VECTORINT64(); len(got) != 2 || got[0] != 9 {
		t.Fatal(got)
	}
}

type recordPush struct {
	mu  sync.Mutex
	got []*mtproto.Updates
}

func (r *recordPush) PushUser(_ context.Context, _ int64, up *mtproto.Updates) error {
	r.mu.Lock()
	r.got = append(r.got, up)
	r.mu.Unlock()
	return nil
}

func (r *recordPush) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.got)
}

func (r *recordPush) updates() []*mtproto.Updates {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*mtproto.Updates(nil), r.got...)
}

func hotPack(id int64, short, title string, masks, emojis bool) *Pack {
	return &Pack{
		ID:               id,
		AccessHash:       id + 1,
		ShortName:        short,
		Title:            title,
		Kind:             KindSticker,
		InstalledDefault: true,
		Masks:            masks,
		Emojis:           emojis,
		Items:            []*Item{stickerItem(id+10, "😀", emojis)},
	}
}
