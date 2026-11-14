package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestParticipantSearchByDisplayName(t *testing.T) {
	SetMemberNames(func(context.Context, int64, []int64) map[int64]string {
		return map[int64]string{2: "ada lovelace", 3: "grace hopper"}
	})
	t.Cleanup(func() { SetMemberNames(nil) })
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Find", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	in := inCh(created.Chats[0])
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	filter := mtproto.MakeTLChannelParticipantsSearch(&mtproto.ChannelParticipantsFilter{Q_STRING: "ada"}).To_ChannelParticipantsFilter()
	got, err := svc.GetParticipants(ctx, 1, in, filter, 0, 50, 0)
	if err != nil || got.GetCount() != 1 || got.GetParticipants()[0].GetUserId() != 2 {
		t.Fatalf("ada %+v %v", got, err)
	}
	none := mtproto.MakeTLChannelParticipantsSearch(&mtproto.ChannelParticipantsFilter{Q_STRING: "zzz"}).To_ChannelParticipantsFilter()
	empty, err := svc.GetParticipants(ctx, 1, in, none, 0, 50, 0)
	if err != nil || empty.GetCount() != 0 {
		t.Fatalf("zzz %+v %v", empty, err)
	}
}
