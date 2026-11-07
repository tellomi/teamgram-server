package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestReadHistoryInboxReachesReaderOtherSessions(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Inbox")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 31,
		Message:  &mtproto.Message{Message: "hello", Date: 30},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := messageID(sent)

	var pushed []pushedUpdate
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, exclude []int64, updates *mtproto.Updates) {
		pushed = append(pushed, pushedUpdate{userID: userID, exclude: append([]int64(nil), exclude...), updates: updates})
	}})

	if err := svc.ReadHistory(ctx, 99, in, msgID); err == nil {
		t.Fatal("non-member read succeeded")
	}
	if len(pushed) != 0 {
		t.Fatalf("non-member pushed %#v", pushed)
	}

	if err := svc.ReadHistory(ctx, 2, in, msgID); err != nil {
		t.Fatal(err)
	}
	if !sawRead(pushed, 2, mtproto.Predicate_updateReadChannelInbox, msgID) {
		t.Fatalf("reader inbox missing %#v", pushed)
	}
	if !sawRead(pushed, 1, mtproto.Predicate_updateReadChannelOutbox, msgID) {
		t.Fatalf("sender outbox missing %#v", pushed)
	}
	inboxN, outboxN := countRead(pushed, 2, mtproto.Predicate_updateReadChannelInbox), countRead(pushed, 1, mtproto.Predicate_updateReadChannelOutbox)
	if err := svc.ReadHistory(ctx, 2, in, msgID); err != nil {
		t.Fatal(err)
	}
	if countRead(pushed, 2, mtproto.Predicate_updateReadChannelInbox) != inboxN {
		t.Fatal("repeat read pushed inbox again")
	}
	if countRead(pushed, 1, mtproto.Predicate_updateReadChannelOutbox) != outboxN {
		t.Fatal("repeat read pushed outbox again")
	}
	reader := dialogRead(t, svc, ctx, 2, ch.Id)
	if reader.ReadInbox < msgID {
		t.Fatalf("reconnected dialog inbox %d", reader.ReadInbox)
	}
}

func sawRead(pushed []pushedUpdate, user int64, pred string, maxID int32) bool {
	return countRead(pushed, user, pred) > 0 && readMax(pushed, user, pred) == maxID
}

func countRead(pushed []pushedUpdate, user int64, pred string) int {
	n := 0
	for _, p := range pushed {
		if p.userID == user && findUpdate(p.updates, pred) != nil {
			n++
		}
	}
	return n
}

func readMax(pushed []pushedUpdate, user int64, pred string) int32 {
	for _, p := range pushed {
		if p.userID != user {
			continue
		}
		if u := findUpdate(p.updates, pred); u != nil {
			return u.GetMaxId()
		}
	}
	return 0
}
