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
