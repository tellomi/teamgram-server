package rpc

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogfilter"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func filter(id int32, title string) *mtproto.DialogFilter {
	return mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
		Id: id,
		Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text:     title,
			Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities(),
		PinnedPeers:  []*mtproto.InputPeer{},
		IncludePeers: []*mtproto.InputPeer{},
		ExcludePeers: []*mtproto.InputPeer{},
	}).To_DialogFilter()
}

func TestDialogFiltersPersistPerUser(t *testing.T) {
	dialogfilter.Use(nil)
	svc := New()
	ctx := withUser(7)
	other := withUser(8)

	if _, err := svc.MessagesUpdateDialogFilter(context.Background(), &mtproto.TLMessagesUpdateDialogFilter{
		Id: 1, Filter: filter(1, "Unread"),
	}); err == nil {
		t.Fatal("write without a user must fail")
	}

	for _, row := range []struct {
		id    int32
		title string
	}{{1, "Unread"}, {2, "Work"}} {
		ok, err := svc.MessagesUpdateDialogFilter(ctx, &mtproto.TLMessagesUpdateDialogFilter{
			Id: row.id, Filter: filter(row.id, row.title),
		})
		if err != nil || !mtproto.FromBool(ok) {
			t.Fatalf("put %d: %v", row.id, err)
		}
	}
	if _, err := svc.MessagesUpdateDialogFilter(ctx, &mtproto.TLMessagesUpdateDialogFilter{
		Id: 3, Filter: filter(9, "mismatch"),
	}); err == nil {
		t.Fatal("filter id must match the request id")
	}

	got, err := svc.MessagesGetDialogFiltersEFD48C89(ctx, &mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetTagsEnabled() || len(got.GetFilters()) != 2 {
		t.Fatalf("got tags=%v len=%d", got.GetTagsEnabled(), len(got.GetFilters()))
	}
	if got.GetFilters()[0].GetTitle_TEXTWITHENTITIES().GetText() != "Unread" {
		t.Fatal(got.GetFilters()[0].GetTitle_TEXTWITHENTITIES().GetText())
	}

	if _, err := svc.MessagesUpdateDialogFiltersOrder(ctx, &mtproto.TLMessagesUpdateDialogFiltersOrder{
		Order: []int32{2, 1},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.MessagesGetDialogFiltersEFD48C89(ctx, &mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetFilters()[0].GetId() != 2 || got.GetFilters()[1].GetId() != 1 {
		t.Fatalf("order = %d %d", got.GetFilters()[0].GetId(), got.GetFilters()[1].GetId())
	}

	if _, err := svc.MessagesToggleDialogFilterTags(ctx, &mtproto.TLMessagesToggleDialogFilterTags{
		Enabled: mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.MessagesGetDialogFiltersEFD48C89(ctx, &mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil || !got.GetTagsEnabled() {
		t.Fatalf("tags: %v %v", got.GetTagsEnabled(), err)
	}

	legacy, err := svc.MessagesGetDialogFiltersF19ED96D(ctx, &mtproto.TLMessagesGetDialogFiltersF19ED96D{})
	if err != nil || len(legacy.GetDatas()) != 2 || legacy.GetDatas()[0].GetId() != 2 {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}

	empty, err := svc.MessagesGetDialogFiltersEFD48C89(other, &mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil || len(empty.GetFilters()) != 0 || empty.GetTagsEnabled() {
		t.Fatalf("other user: %+v %v", empty, err)
	}

	if _, err := svc.MessagesUpdateDialogFilter(ctx, &mtproto.TLMessagesUpdateDialogFilter{Id: 2}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.MessagesGetDialogFiltersEFD48C89(ctx, &mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil || len(got.GetFilters()) != 1 || got.GetFilters()[0].GetId() != 1 {
		t.Fatalf("after delete: %+v %v", got.GetFilters(), err)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}
