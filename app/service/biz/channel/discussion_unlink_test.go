package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDiscussionUnlinkAndRebindClearsGroup(t *testing.T) {
	svc, ctx := newTest(t)
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Desk", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	b := broadcast.Chats[0]
	hall := megagroup(t, svc, ctx, "Hall")
	yard := megagroup(t, svc, ctx, "Yard")
	if _, err := svc.Invite(ctx, 1, 0, inCh(hall), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}

	var pushed []pushedUpdate
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, exclude []int64, updates *mtproto.Updates) {
		pushed = append(pushed, pushedUpdate{userID: userID, exclude: append([]int64(nil), exclude...), updates: updates})
	}})

	if err := svc.SetDiscussionGroup(ctx, 1, 9, inCh(b), inCh(hall)); err != nil {
		t.Fatal(err)
	}
	if got := linkedID(t, svc, ctx, b); got != hall.Id {
		t.Fatalf("channel link %d", got)
	}
	if got := linkedID(t, svc, ctx, hall); got != b.Id {
		t.Fatalf("hall link %d", got)
	}
	if !sawChannelPush(pushed, 2, hall.Id, nil) || !sawChannelPush(pushed, 1, b.Id, []int64{9}) {
		t.Fatalf("link fanout %#v", pushed)
	}

	pushed = nil
	if err := svc.SetDiscussionGroup(ctx, 1, 9, inCh(b), inCh(yard)); err != nil {
		t.Fatal(err)
	}
	if got := linkedID(t, svc, ctx, b); got != yard.Id {
		t.Fatalf("rebind channel %d", got)
	}
	if got := linkedID(t, svc, ctx, hall); got != 0 {
		t.Fatalf("hall still linked %d", got)
	}
	if got := linkedID(t, svc, ctx, yard); got != b.Id {
		t.Fatalf("yard link %d", got)
	}
	if !sawChannelPush(pushed, 2, hall.Id, nil) || !sawChannelPush(pushed, 1, yard.Id, []int64{9}) || !sawChannelPush(pushed, 1, b.Id, []int64{9}) {
		t.Fatalf("rebind fanout %#v", pushed)
	}

	pushed = nil
	empty := mtproto.MakeTLInputChannelEmpty(&mtproto.InputChannel{}).To_InputChannel()
	if err := svc.SetDiscussionGroup(ctx, 1, 9, inCh(b), empty); err != nil {
		t.Fatal(err)
	}
	if got := linkedID(t, svc, ctx, b); got != 0 {
		t.Fatalf("unlinked channel %d", got)
	}
	if got := linkedID(t, svc, ctx, yard); got != 0 {
		t.Fatalf("yard still linked %d", got)
	}
	if got := linkedID(t, svc, ctx, hall); got != 0 {
		t.Fatalf("hall changed %d", got)
	}
	if !sawChannelPush(pushed, 1, yard.Id, []int64{9}) || !sawChannelPush(pushed, 1, b.Id, []int64{9}) {
		t.Fatalf("unlink fanout %#v", pushed)
	}
}

func TestDiscussionUnlinkLeavesForeignBacklink(t *testing.T) {
	svc, ctx := newTest(t)
	first, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "DeskOne", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "DeskTwo", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	a := first.Chats[0]
	b := second.Chats[0]
	hall := megagroup(t, svc, ctx, "HallTwo")
	if err := svc.SetDiscussionGroup(ctx, 1, 0, inCh(a), inCh(hall)); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDiscussionGroup(ctx, 1, 0, inCh(b), inCh(hall)); err != nil {
		t.Fatal(err)
	}
	empty := mtproto.MakeTLInputChannelEmpty(&mtproto.InputChannel{}).To_InputChannel()
	if err := svc.SetDiscussionGroup(ctx, 1, 0, inCh(a), empty); err != nil {
		t.Fatal(err)
	}
	if got := linkedID(t, svc, ctx, a); got != 0 {
		t.Fatalf("first channel %d", got)
	}
	if got := linkedID(t, svc, ctx, hall); got != b.Id {
		t.Fatalf("hall backlink %d, want %d", got, b.Id)
	}
	if got := linkedID(t, svc, ctx, b); got != hall.Id {
		t.Fatalf("second channel %d", got)
	}
}

func linkedID(t *testing.T, svc *Service, ctx context.Context, ch *mtproto.Chat) int64 {
	t.Helper()
	full, err := svc.GetFullChannel(ctx, 1, inCh(ch))
	if err != nil {
		t.Fatal(err)
	}
	return full.FullChat.GetLinkedChatId().GetValue()
}

func sawChannelPush(pushed []pushedUpdate, userID, channelID int64, exclude []int64) bool {
	for _, p := range pushed {
		if p.userID != userID || len(p.exclude) != len(exclude) {
			continue
		}
		match := true
		for i := range exclude {
			if p.exclude[i] != exclude[i] {
				match = false
				break
			}
		}
		if !match || p.updates == nil {
			continue
		}
		for _, u := range p.updates.GetUpdates() {
			if u.GetPredicateName() == mtproto.Predicate_updateChannel && u.GetChannelId() == channelID {
				return true
			}
		}
	}
	return false
}
