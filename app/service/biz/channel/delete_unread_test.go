package channel

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

func TestDuplicateDeleteIDDecrementsOnce(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Dup")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	id := channelText(t, svc, ctx, ch.Id, 2, 41, "once")
	before := dialogRead(t, svc, ctx, 1, ch.Id).Unread
	if before < 1 {
		t.Fatalf("unread before %d", before)
	}
	if _, count, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{id, id}); err != nil || count != 1 {
		t.Fatalf("duplicate delete count %d %v", count, err)
	}
	if got := dialogRead(t, svc, ctx, 1, ch.Id).Unread; got != before-1 {
		t.Fatalf("duplicate id unread %d, want %d", got, before-1)
	}
	if _, count, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{id}); err != nil || count != 0 {
		t.Fatalf("second delete count %d %v", count, err)
	}
	if got := dialogRead(t, svc, ctx, 1, ch.Id).Unread; got != before-1 {
		t.Fatalf("already-deleted id changed unread to %d, want %d", got, before-1)
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

func TestDeleteUnreadPushesOnlyOnlineMembers(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Online")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	id := channelText(t, svc, ctx, ch.Id, 2, 21, "live")
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 22, Message: &mtproto.Message{Message: "still"}}}); err != nil {
		t.Fatal(err)
	}
	before3 := dialogRead(t, svc, ctx, 3, ch.Id).Unread
	var pushed []pushedUpdate
	svc.SetHooks(Hooks{
		Online: func(context.Context, []int64) (map[int64]bool, error) {
			return map[int64]bool{1: true}, nil
		},
		Push: func(_ context.Context, userID int64, exclude []int64, updates *mtproto.Updates) {
			pushed = append(pushed, pushedUpdate{userID: userID, exclude: append([]int64(nil), exclude...), updates: updates})
		},
	})
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	if !inboxStill(pushed, 1, dialogRead(t, svc, ctx, 1, ch.Id).Unread) {
		t.Fatal("online member was not told the new count")
	}
	if inboxPushed(pushed, 3) {
		t.Fatal("offline member was pushed")
	}
	if got := dialogRead(t, svc, ctx, 3, ch.Id).Unread; got != before3-1 {
		t.Fatalf("offline unread %d, was %d", got, before3)
	}
}

func TestLargeGroupDeleteUnreadDoesNotFanOut(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Bulk")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	id := channelText(t, svc, ctx, ch.Id, 2, 31, "bulk")
	for uid := int64(100); uid < 100+1000; uid++ {
		if err := svc.store.PutParticipant(ctx, &Participant{ChannelID: ch.Id, UserID: uid, State: StateNormal}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := svc.store.CountMembers(ctx, ch.Id)
	if err != nil || n <= unreadSyncCap {
		t.Fatalf("members %d %v", n, err)
	}
	before := dialogRead(t, svc, ctx, 1, ch.Id).Unread
	var inbox int
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
		if findUpdate(updates, mtproto.Predicate_updateReadChannelInbox) != nil {
			inbox++
		}
	}})
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	if inbox != 0 {
		t.Fatalf("large group pushed %d inbox updates on the request", inbox)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if dialogRead(t, svc, ctx, 1, ch.Id).Unread == before-1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("unread stayed %d, want %d", dialogRead(t, svc, ctx, 1, ch.Id).Unread, before-1)
}

func TestUnreadCutSQLMatchesTheInboxIndex(t *testing.T) {
	for _, piece := range []string{
		"GREATEST(unread_count - ?, 0)",
		"channel_id = ?",
		"read_inbox_max_id < ?",
		"user_id <> ?",
		"unread_count > 0",
	} {
		if !strings.Contains(unreadCutUpdateSQL, piece) {
			t.Fatalf("update SQL missing %s", piece)
		}
	}
	if !strings.Contains(unreadCutSelectSQL, "unread_count > 0") || !strings.Contains(unreadCutSelectSQL, "read_inbox_max_id < ?") {
		t.Fatal("select SQL does not use the update predicate")
	}
	if !strings.Contains(schemaSQL, "KEY channel_read_inbox (channel_id, read_inbox_max_id)") {
		t.Fatal("new tables do not create channel_read_inbox")
	}
	raw, err := os.ReadFile("../../../../teamgramd/deploy/sql/migrate-20261129-channel-read-inbox.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "teamgram_add_index('channel_read_state', 'channel_read_inbox'") {
		t.Fatal("migration does not add the inbox index")
	}
	if strings.Contains(strings.ToUpper(text), "DROP ") || strings.Contains(strings.ToUpper(text), "TRUNCATE") {
		t.Fatal("migration deletes data")
	}
}

func inboxPushed(pushed []pushedUpdate, user int64) bool {
	for _, p := range pushed {
		if p.userID == user && findUpdate(p.updates, mtproto.Predicate_updateReadChannelInbox) != nil {
			return true
		}
	}
	return false
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
