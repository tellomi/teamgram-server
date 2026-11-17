package service

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestStickerOpsPushOtherSessions(t *testing.T) {
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	catalog.Open("")
	fake := &fakeSync{}
	svc := New(fake)
	ctx := withStickerUser(7, 70)
	set := mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: catalog.ShortTwemoji}).To_InputStickerSet()
	if _, err := svc.MessagesInstallStickerSet(ctx, &mtproto.TLMessagesInstallStickerSet{Stickerset: set}); err != nil {
		t.Fatal(err)
	}
	install := fake.last()
	if install.GetUserId() != 7 || install.GetPermAuthKeyId() != 70 {
		t.Fatalf("install target %#v", install)
	}
	up := install.GetUpdates().GetUpdates()[0]
	if up.GetPredicateName() != mtproto.Predicate_updateNewStickerSet || up.GetStickerset_MESSAGES_STICKERSET() == nil {
		t.Fatalf("install update %#v", up)
	}

	full, err := catalog.Must().GetSet(7, set, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.MessagesReorderStickerSets(ctx, &mtproto.TLMessagesReorderStickerSets{Order: []int64{full.GetSet().GetId()}}); err != nil {
		t.Fatal(err)
	}
	order := fake.last().GetUpdates().GetUpdates()[0]
	if order.GetPredicateName() != mtproto.Predicate_updateStickerSetsOrder || len(order.GetOrder_VECTORINT64()) != 1 {
		t.Fatalf("order %#v", order)
	}

	if _, err = svc.MessagesToggleStickerSets(ctx, &mtproto.TLMessagesToggleStickerSets{
		Archive:     true,
		Stickersets: []*mtproto.InputStickerSet{set},
	}); err != nil {
		t.Fatal(err)
	}
	archived := fake.last().GetUpdates().GetUpdates()[0]
	if archived.GetPredicateName() != mtproto.Predicate_updateStickerSets || archived.GetMasks() || archived.GetEmojis() {
		t.Fatalf("archive %#v", archived)
	}

	if _, err = svc.MessagesUninstallStickerSet(ctx, &mtproto.TLMessagesUninstallStickerSet{Stickerset: set}); err != nil {
		t.Fatal(err)
	}
	removed := fake.last().GetUpdates().GetUpdates()[0]
	if removed.GetPredicateName() != mtproto.Predicate_updateStickerSets {
		t.Fatalf("uninstall %#v", removed)
	}
}

func withStickerUser(userID, perm int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: perm}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

type fakeSync struct {
	mu  sync.Mutex
	got []*syncpb.TLSyncUpdatesNotMe
}

func (f *fakeSync) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	f.mu.Lock()
	f.got = append(f.got, in)
	f.mu.Unlock()
	return mtproto.EmptyVoid, nil
}

func (f *fakeSync) last() *syncpb.TLSyncUpdatesNotMe {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.got) == 0 {
		return nil
	}
	return f.got[len(f.got)-1]
}

func (f *fakeSync) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.got)
}

// TestPersonalStickerListsPushOtherSessions is ISSUE-S-STICKER-SYNC-PUSH-001.
// saveGif, saveRecentSticker, clearRecentStickers, and faveSticker push the
// matching update through SyncUpdatesNotMe, which addresses the caller's
// perm auth key so that session is left out. A failed write and a no-op
// (already first, or nothing to clear) do not push.
func TestPersonalStickerListsPushOtherSessions(t *testing.T) {
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	catalog.Open("")
	fake := &fakeSync{}
	svc := New(fake)
	const user, perm int64 = 880051, 880510
	ctx := withStickerUser(user, perm)

	gifs := catalog.BundledGifs(0).GetGifs()
	if len(gifs) == 0 {
		t.Fatal("no bundled gif")
	}
	gif := inputDocument(gifs[len(gifs)-1])
	if _, err := svc.MessagesSaveGif(ctx, &mtproto.TLMessagesSaveGif{
		Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{}).To_InputDocument(),
	}); err == nil {
		t.Fatal("empty gif id succeeded")
	}
	if fake.count() != 0 {
		t.Fatal("failed saveGif pushed")
	}
	if _, err := svc.MessagesSaveGif(ctx, &mtproto.TLMessagesSaveGif{Id: gif}); err != nil {
		t.Fatal(err)
	}
	assertNotMe(t, fake.last(), user, perm, mtproto.Predicate_updateSavedGifs)
	if _, err := svc.MessagesSaveGif(ctx, &mtproto.TLMessagesSaveGif{Id: gif}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 1 {
		t.Fatalf("saving the gif that is already first pushed again: %d", fake.count())
	}
	if _, err := svc.MessagesSaveGif(ctx, &mtproto.TLMessagesSaveGif{Id: gif, Unsave: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	assertNotMe(t, fake.last(), user, perm, mtproto.Predicate_updateSavedGifs)
	n := fake.count()
	if _, err := svc.MessagesSaveGif(ctx, &mtproto.TLMessagesSaveGif{Id: gif, Unsave: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n {
		t.Fatal("unsaving a gif that is not saved pushed")
	}

	full, err := catalog.Must().GetSet(user, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{ShortName: catalog.ShortTwemoji}).To_InputStickerSet(), 0)
	if err != nil || len(full.GetDocuments()) == 0 {
		t.Fatalf("twemoji: %v", err)
	}
	sticker := inputDocument(full.GetDocuments()[0])
	if _, err = svc.MessagesSaveRecentSticker(ctx, &mtproto.TLMessagesSaveRecentSticker{
		Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 1}).To_InputDocument(),
	}); err == nil {
		t.Fatal("unknown recent sticker succeeded")
	}
	if fake.count() != n {
		t.Fatal("failed saveRecentSticker pushed")
	}
	if _, err = svc.MessagesSaveRecentSticker(ctx, &mtproto.TLMessagesSaveRecentSticker{Id: sticker}); err != nil {
		t.Fatal(err)
	}
	assertNotMe(t, fake.last(), user, perm, mtproto.Predicate_updateRecentStickers)
	if _, err = svc.MessagesSaveRecentSticker(ctx, &mtproto.TLMessagesSaveRecentSticker{Id: sticker}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n+1 {
		t.Fatalf("saving the recent sticker that is already first pushed again: %d", fake.count())
	}
	if _, err = svc.MessagesClearRecentStickers(ctx, &mtproto.TLMessagesClearRecentStickers{Attached: true}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n+1 {
		t.Fatal("clearing an empty attached recent list pushed")
	}
	if _, err = svc.MessagesClearRecentStickers(ctx, &mtproto.TLMessagesClearRecentStickers{}); err != nil {
		t.Fatal(err)
	}
	assertNotMe(t, fake.last(), user, perm, mtproto.Predicate_updateRecentStickers)
	if _, err = svc.MessagesClearRecentStickers(ctx, &mtproto.TLMessagesClearRecentStickers{}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n+2 {
		t.Fatal("clearing an empty recent list pushed")
	}

	if _, err = svc.MessagesFaveSticker(ctx, &mtproto.TLMessagesFaveSticker{
		Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 1}).To_InputDocument(),
	}); err == nil {
		t.Fatal("unknown fave succeeded")
	}
	if fake.count() != n+2 {
		t.Fatal("failed faveSticker pushed")
	}
	if _, err = svc.MessagesFaveSticker(ctx, &mtproto.TLMessagesFaveSticker{Id: sticker}); err != nil {
		t.Fatal(err)
	}
	assertNotMe(t, fake.last(), user, perm, mtproto.Predicate_updateFavedStickers)
	if _, err = svc.MessagesFaveSticker(ctx, &mtproto.TLMessagesFaveSticker{Id: sticker}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n+3 {
		t.Fatal("faving the sticker that is already first pushed again")
	}
	if _, err = svc.MessagesFaveSticker(ctx, &mtproto.TLMessagesFaveSticker{Id: sticker, Unfave: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	assertNotMe(t, fake.last(), user, perm, mtproto.Predicate_updateFavedStickers)
	if _, err = svc.MessagesFaveSticker(ctx, &mtproto.TLMessagesFaveSticker{Id: sticker, Unfave: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n+4 {
		t.Fatal("unfaving a sticker that is not faved pushed")
	}
}

func inputDocument(doc *mtproto.Document) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:            doc.GetId(),
		AccessHash:    doc.GetAccessHash(),
		FileReference: append([]byte(nil), doc.GetFileReference()...),
	}).To_InputDocument()
}

func assertNotMe(t *testing.T, got *syncpb.TLSyncUpdatesNotMe, user, perm int64, predicate string) {
	t.Helper()
	if got == nil || got.GetUserId() != user || got.GetPermAuthKeyId() != perm {
		t.Fatalf("not-me target %#v", got)
	}
	ups := got.GetUpdates().GetUpdates()
	if len(ups) != 1 || ups[0].GetPredicateName() != predicate {
		t.Fatalf("update %s, got %#v", predicate, ups)
	}
}
