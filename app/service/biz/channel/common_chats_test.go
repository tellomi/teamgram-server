package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCommonMegagroupsSkipBroadcastAndOutsider(t *testing.T) {
	svc, ctx := newTest(t)
	group := megagroup(t, svc, ctx, "Together")
	if _, err := svc.Invite(ctx, 1, 0, inCh(group), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, 1, 0, inCh(news.Chats[0]), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	alone := megagroup(t, svc, ctx, "Alone")
	got, err := svc.CommonMegagroups(ctx, 1, 2)
	if err != nil || len(got) != 1 || got[0].GetId() != group.Id || !got[0].GetMegagroup() {
		t.Fatalf("%v %#v", err, got)
	}
	if _, err := svc.CommonMegagroups(ctx, 1, 1); err != mtproto.ErrUserIdInvalid {
		t.Fatal(err)
	}
	none, err := svc.CommonMegagroups(ctx, 1, 3)
	if err != nil || len(none) != 0 {
		t.Fatalf("outsider %v %#v", err, none)
	}
	_ = alone
}
