package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDeleteOthersUnreadDropsCountAndPushesInbox(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Unread")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	first := channelText(t, svc, ctx, ch.Id, 2, 11, "one")
	second := channelText(t, svc, ctx, ch.Id, 2, 12, "two")
	if got := dialogRead(t, svc, ctx, 1, ch.Id).Unread; got != 2 {
		t.Fatalf("member unread before delete %d", got)
	}
	// The invite service message is unread for user 3 and already read by
	// the creator who sent it, so user 3 starts one ahead of user 1.
	base3 := dialogRead(t, svc, ctx, 3, ch.Id).Unread
	if base3 != 3 {
		t.Fatalf("other member unread %d", base3)
	}
	if got := dialogRead(t, svc, ctx, 2, ch.Id).Unread; got != 0 {
		t.Fatalf("author unread %d", got)
	}

	var pushed []pushedUpdate
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, exclude []int64, updates *mtproto.Updates) {
		pushed = append(pushed, pushedUpdate{userID: userID, exclude: append([]int64(nil), exclude...), updates: updates})
	}})

	if _, count, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{first}); err != nil || count != 1 {
		t.Fatalf("delete count %d %v", count, err)
	}
	if got := dialogRead(t, svc, ctx, 1, ch.Id).Unread; got != 1 {
		t.Fatalf("member unread after one delete %d", got)
	}
	if got := dialogRead(t, svc, ctx, 3, ch.Id).Unread; got != base3-1 {
		t.Fatalf("other member unread after one delete %d", got)
	}
	if got := dialogRead(t, svc, ctx, 2, ch.Id).Unread; got != 0 {
		t.Fatalf("author unread changed to %d", got)
	}
	if !inboxStill(pushed, 1, 1) || !inboxStill(pushed, 3, base3-1) {
		t.Fatalf("missing still_unread_count=1 %#v", pushed)
	}
	if inboxStill(pushed, 2, 0) || inboxStill(pushed, 2, 1) {
		t.Fatal("author was told their own message was unread")
	}

	if err := svc.ReadHistory(ctx, 1, in, second); err != nil {
		t.Fatal(err)
	}
	if got := dialogRead(t, svc, ctx, 1, ch.Id).Unread; got != 0 {
		t.Fatalf("after read %d", got)
	}
	before := dialogRead(t, svc, ctx, 3, ch.Id).Unread
	pushed = nil
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{second}); err != nil {
		t.Fatal(err)
	}
	if got := dialogRead(t, svc, ctx, 1, ch.Id).Unread; got != 0 {
		t.Fatalf("already-read delete changed unread to %d", got)
	}
	if got := dialogRead(t, svc, ctx, 3, ch.Id).Unread; got != before-1 {
		t.Fatalf("unread member %d, was %d", got, before)
	}
	if !inboxStill(pushed, 3, before-1) {
		t.Fatalf("still_unread for the member who had not read %#v", pushed)
	}
	for _, p := range pushed {
		if p.userID == 1 && findUpdate(p.updates, mtproto.Predicate_updateReadChannelInbox) != nil {
			t.Fatal("already-read member got a new inbox count")
		}
	}
}

func channelText(t *testing.T, svc *Service, ctx context.Context, channelID, sender, random int64, text string) int32 {
	t.Helper()
	sent, err := svc.Send(ctx, sender, channelID, 0, []Outgoing{{
		RandomID: random,
		Message:  &mtproto.Message{Message: text},
	}})
	if err != nil {
		t.Fatal(err)
	}
	id := messageID(sent)
	if id == 0 {
		t.Fatal("no message id")
	}
	return id
}

func inboxStill(pushed []pushedUpdate, user int64, still int32) bool {
	for _, p := range pushed {
		if p.userID != user {
			continue
		}
		u := findUpdate(p.updates, mtproto.Predicate_updateReadChannelInbox)
		if u != nil && u.GetStillUnreadCount() == still && u.GetPts_INT32() > 0 {
			return true
		}
	}
	return false
}
