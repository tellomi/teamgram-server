package channel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestUnpinAllProtocol is the in-memory run of the channel pin protocol.
// The same checks run against MySQL in TestUnpinAllProtocolMySQL.
func TestUnpinAllProtocol(t *testing.T) {
	svc, ctx := newTest(t)
	checkUnpinProtocol(t, svc, ctx)
}

func checkUnpinProtocol(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	checkMegagroupPins(t, svc, ctx)
	checkBroadcastPins(t, svc, ctx)
	checkForumTopicPins(t, svc, ctx)
	checkPinPermission(t, svc, ctx)
}

func checkMegagroupPins(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	ch := mustCreate(t, svc, ctx, CreateRequest{CreatorID: 1, Title: "Pins", Megagroup: true})
	one := mustSend(t, svc, ctx, ch.Id, 11, "one", nil)
	two := mustSend(t, svc, ctx, ch.Id, 12, "two", nil)
	three := mustSend(t, svc, ctx, ch.Id, 13, "three", nil)
	mustPin(t, svc, ctx, ch.Id, one, false)
	mustPin(t, svc, ctx, ch.Id, two, false)
	mustPin(t, svc, ctx, ch.Id, three, false)
	mustPin(t, svc, ctx, ch.Id, two, true)
	if historyPinned(t, svc, ctx, ch, two) {
		t.Fatal("single unpin left the message pinned")
	}
	if !historyPinned(t, svc, ctx, ch, one) || !historyPinned(t, svc, ctx, ch, three) {
		t.Fatal("single unpin cleared a different message")
	}
	if got := pinnedIDs(t, svc, ctx, ch.Id, 0); len(got) != 2 || !containsAll(got, one, three) {
		t.Fatalf("pinned search after one unpin: %v", got)
	}
	if id := fullPinned(t, svc, ctx, ch); id != three {
		t.Fatalf("feature pin id %d, want the last pin %d", id, three)
	}

	before := mustReload(t, svc, ctx, ch.Id)
	pts, count, err := svc.UnpinAll(ctx, 1, 0, ch.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertPinEdits(t, svc, ctx, ch, before.Pts, pts, count, []int32{one, three})
	if len(pinnedIDs(t, svc, ctx, ch.Id, 0)) != 0 {
		t.Fatal("megagroup unpin-all left a pinned search hit")
	}
	if historyPinned(t, svc, ctx, ch, one) || historyPinned(t, svc, ctx, ch, three) {
		t.Fatal("megagroup history still has pinned")
	}
	if id := fullPinned(t, svc, ctx, ch); id != 0 {
		t.Fatalf("feature pin id %d after unpin-all", id)
	}
	again, err := svc.reload(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	pts2, count2, err := svc.UnpinAll(ctx, 1, 0, ch.Id, 0)
	if err != nil || count2 != 0 || pts2 != again.Pts {
		t.Fatalf("second unpin pts=%d count=%d channel=%d err=%v", pts2, count2, again.Pts, err)
	}
}

func checkBroadcastPins(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	ch := mustCreate(t, svc, ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	one := mustSend(t, svc, ctx, ch.Id, 21, "post-one", nil)
	two := mustSend(t, svc, ctx, ch.Id, 22, "post-two", nil)
	mustPin(t, svc, ctx, ch.Id, one, false)
	mustPin(t, svc, ctx, ch.Id, two, false)
	before := mustReload(t, svc, ctx, ch.Id)
	pts, count, err := svc.UnpinAll(ctx, 1, 0, ch.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertPinEdits(t, svc, ctx, ch, before.Pts, pts, count, []int32{one, two})
	if len(pinnedIDs(t, svc, ctx, ch.Id, 0)) != 0 || historyPinned(t, svc, ctx, ch, one) || historyPinned(t, svc, ctx, ch, two) {
		t.Fatal("broadcast unpin-all left a pin")
	}
}

func checkForumTopicPins(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	ch := mustCreate(t, svc, ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	_, left, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Left", 11, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, right, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Right", 22, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftReply := topicReply(left.ID)
	rightReply := topicReply(right.ID)
	leftA := mustSend(t, svc, ctx, ch.Id, 31, "left-a", leftReply)
	leftB := mustSend(t, svc, ctx, ch.Id, 32, "left-b", leftReply)
	rightA := mustSend(t, svc, ctx, ch.Id, 33, "right-a", rightReply)
	general := mustSend(t, svc, ctx, ch.Id, 34, "general-note", nil)
	kept := mustSend(t, svc, ctx, ch.Id, 35, "keep-this-text", rightReply)
	mustPin(t, svc, ctx, ch.Id, left.ID, false)
	mustPin(t, svc, ctx, ch.Id, leftA, false)
	mustPin(t, svc, ctx, ch.Id, leftB, false)
	mustPin(t, svc, ctx, ch.Id, rightA, false)
	mustPin(t, svc, ctx, ch.Id, general, false)
	if err := svc.ReorderPinnedTopics(ctx, 1, ch.Id, []int32{right.ID, left.ID}, true); err != nil {
		t.Fatal(err)
	}
	topicsBefore := topicSnap(t, svc, ctx, ch.Id)

	before := mustReload(t, svc, ctx, ch.Id)
	pts, count, err := svc.UnpinAll(ctx, 1, 0, ch.Id, left.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertPinEdits(t, svc, ctx, ch, before.Pts, pts, count, []int32{left.ID, leftA, leftB})
	if got := pinnedIDs(t, svc, ctx, ch.Id, left.ID); len(got) != 0 {
		t.Fatalf("left topic still pinned: %v", got)
	}
	if got := pinnedIDs(t, svc, ctx, ch.Id, right.ID); len(got) != 1 || got[0] != rightA {
		t.Fatalf("right topic pins: %v", got)
	}
	if got := pinnedIDs(t, svc, ctx, ch.Id, generalTopicID); len(got) != 1 || got[0] != general {
		t.Fatalf("general pins: %v", got)
	}
	if !historyPinned(t, svc, ctx, ch, rightA) || !historyPinned(t, svc, ctx, ch, general) {
		t.Fatal("unpin of the left topic cleared another topic")
	}
	if historyPinned(t, svc, ctx, ch, leftA) || historyPinned(t, svc, ctx, ch, leftB) || historyPinned(t, svc, ctx, ch, left.ID) {
		t.Fatal("left topic message stayed pinned")
	}
	if text := historyText(t, svc, ctx, ch, kept); text != "keep-this-text" {
		t.Fatalf("unrelated message text %q", text)
	}
	if id := fullPinned(t, svc, ctx, ch); id != general {
		t.Fatalf("feature pin %d, want general %d", id, general)
	}
	if topicsAfter := topicSnap(t, svc, ctx, ch.Id); topicsAfter != topicsBefore {
		t.Fatalf("topic rows changed\nbefore %s\nafter  %s", topicsBefore, topicsAfter)
	}

	before = mustReload(t, svc, ctx, ch.Id)
	pts, count, err = svc.UnpinAll(ctx, 1, 0, ch.Id, generalTopicID)
	if err != nil {
		t.Fatal(err)
	}
	assertPinEdits(t, svc, ctx, ch, before.Pts, pts, count, []int32{general})
	if historyPinned(t, svc, ctx, ch, general) || !historyPinned(t, svc, ctx, ch, rightA) {
		t.Fatal("general unpin changed the right topic")
	}
	if id := fullPinned(t, svc, ctx, ch); id != 0 {
		t.Fatalf("feature pin %d after general unpin, want 0 while right stays pinned in the topic", id)
	}
	if got := pinnedIDs(t, svc, ctx, ch.Id, right.ID); len(got) != 1 || got[0] != rightA {
		t.Fatalf("right topic after general unpin: %v", got)
	}
}

func checkPinPermission(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	ch := mustCreate(t, svc, ctx, CreateRequest{CreatorID: 1, Title: "Rights", Megagroup: true})
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	msg := mustSend(t, svc, ctx, ch.Id, 41, "locked", nil)
	mustPin(t, svc, ctx, ch.Id, msg, false)
	before := mustReload(t, svc, ctx, ch.Id)

	if _, _, err := svc.UnpinAll(ctx, 2, 0, ch.Id, 0); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member unpin: %v", err)
	}
	if _, _, err := svc.UnpinAll(ctx, 9, 0, ch.Id, 0); err != mtproto.ErrChannelPrivate {
		t.Fatalf("outsider unpin: %v", err)
	}
	noPin := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{DeleteMessages: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 4, noPin, "mod"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.UnpinAll(ctx, 4, 0, ch.Id, 0); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("admin without pin right: %v", err)
	}
	if !historyPinned(t, svc, ctx, ch, msg) || len(pinnedIDs(t, svc, ctx, ch.Id, 0)) != 1 {
		t.Fatal("rejected unpin changed the pin")
	}
	now, err := svc.reload(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if now.Pts != before.Pts {
		t.Fatalf("rejected unpin moved pts %d -> %d", before.Pts, now.Pts)
	}

	withPin := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PinMessages: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, withPin, "pin"); err != nil {
		t.Fatal(err)
	}
	pts, count, err := svc.UnpinAll(ctx, 3, 0, ch.Id, 0)
	if err != nil || count != 1 || pts <= before.Pts {
		t.Fatalf("pin admin pts=%d count=%d err=%v", pts, count, err)
	}
	if historyPinned(t, svc, ctx, ch, msg) {
		t.Fatal("pin admin unpin left the message pinned")
	}
}

func assertPinEdits(t *testing.T, svc *Service, ctx context.Context, ch *mtproto.Chat, before, pts, count int32, ids []int32) {
	t.Helper()
	if count != int32(len(ids)) {
		t.Fatalf("pts_count %d, cleared ids %v", count, ids)
	}
	if pts != before+count {
		t.Fatalf("pts %d, before %d, count %d", pts, before, count)
	}
	now := mustReload(t, svc, ctx, ch.Id)
	if now.Pts != pts {
		t.Fatalf("channel pts %d, returned %d", now.Pts, pts)
	}
	diff, err := svc.Difference(ctx, 1, inCh(ch), before, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.GetPts() != pts {
		t.Fatalf("difference pts %d, returned %d", diff.GetPts(), pts)
	}
	if len(diff.GetNewMessages()) != 0 {
		t.Fatalf("unpin wrote new messages: %d", len(diff.GetNewMessages()))
	}
	seen := map[int32]bool{}
	var sum int32
	var last int32
	for _, u := range diff.GetOtherUpdates() {
		if u.GetPredicateName() != mtproto.Predicate_updateEditChannelMessage {
			t.Fatalf("unexpected update %s", u.GetPredicateName())
		}
		if u.GetPtsCount() != 1 {
			t.Fatalf("edit pts_count %d", u.GetPtsCount())
		}
		sum += u.GetPtsCount()
		last = u.GetPts_INT32()
		msg := u.GetMessage_MESSAGE()
		if msg.GetPinned() {
			t.Fatalf("edit %d still pinned", msg.GetId())
		}
		seen[msg.GetId()] = true
	}
	if int(sum) != len(ids) || last != pts {
		t.Fatalf("edits sum=%d last=%d want %d pts %d ids %v seen %v", sum, last, len(ids), pts, ids, seen)
	}
	for _, id := range ids {
		if !seen[id] {
			t.Fatalf("difference missed edit of %d, saw %v", id, seen)
		}
	}
}

func mustCreate(t *testing.T, svc *Service, ctx context.Context, in CreateRequest) *mtproto.Chat {
	t.Helper()
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return created.Chats[0]
}

func mustSend(t *testing.T, svc *Service, ctx context.Context, channelID, random int64, text string, reply *mtproto.MessageReplyHeader) int32 {
	t.Helper()
	sent, err := svc.Send(ctx, 1, channelID, 0, []Outgoing{{
		RandomID: random,
		Message:  &mtproto.Message{Message: text, Date: int32(random), ReplyTo: reply},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return sentChannelMessage(t, sent).GetId()
}

func mustPin(t *testing.T, svc *Service, ctx context.Context, channelID int64, id int32, unpin bool) {
	t.Helper()
	if _, err := svc.PinMessage(ctx, 1, 0, channelID, id, unpin); err != nil {
		t.Fatal(err)
	}
}

func mustReload(t *testing.T, svc *Service, ctx context.Context, channelID int64) *Channel {
	t.Helper()
	ch, err := svc.reload(ctx, channelID)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func topicReply(top int32) *mtproto.MessageReplyHeader {
	return mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: top,
		ReplyToTopId: &wrapperspb.Int32Value{Value: top},
		ForumTopic:   true,
	}).To_MessageReplyHeader()
}

func pinnedFilter() *mtproto.MessagesFilter {
	return mtproto.MakeTLInputMessagesFilterPinned(&mtproto.MessagesFilter{}).To_MessagesFilter()
}

func pinnedIDs(t *testing.T, svc *Service, ctx context.Context, channelID int64, top int32) []int32 {
	t.Helper()
	found, err := svc.SearchInChannel(ctx, 1, channelID, ChannelSearch{Filter: pinnedFilter(), Limit: 50, TopMsgID: top})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int32
	for _, m := range found.GetMessages() {
		ids = append(ids, m.GetId())
	}
	return ids
}

func historyMessage(t *testing.T, svc *Service, ctx context.Context, ch *mtproto.Chat, id int32) *mtproto.Message {
	t.Helper()
	hist, err := svc.History(ctx, 1, inCh(ch), HistoryQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hist.GetMessages() {
		if m.GetId() == id {
			return m
		}
	}
	t.Fatalf("message %d is not in history", id)
	return nil
}

func historyPinned(t *testing.T, svc *Service, ctx context.Context, ch *mtproto.Chat, id int32) bool {
	t.Helper()
	return historyMessage(t, svc, ctx, ch, id).GetPinned()
}

func historyText(t *testing.T, svc *Service, ctx context.Context, ch *mtproto.Chat, id int32) string {
	t.Helper()
	return historyMessage(t, svc, ctx, ch, id).GetMessage()
}

func fullPinned(t *testing.T, svc *Service, ctx context.Context, ch *mtproto.Chat) int32 {
	t.Helper()
	full, err := svc.GetFullChannel(ctx, 1, inCh(ch))
	if err != nil {
		t.Fatal(err)
	}
	return full.GetFullChat().GetPinnedMsgId().GetValue()
}

func topicSnap(t *testing.T, svc *Service, ctx context.Context, channelID int64) string {
	t.Helper()
	topics, _, err := svc.ListTopics(ctx, 1, channelID, "", 20, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	parts := make([]string, 0, len(topics))
	for _, topic := range topics {
		parts = append(parts, fmt.Sprintf("%d/%s/color=%d/pin=%t/top=%d",
			topic.GetId(), topic.GetTitle(), topic.GetIconColor(), topic.GetPinned(), topic.GetTopMessage()))
	}
	return strings.Join(parts, ";")
}

func containsAll(got []int32, want ...int32) bool {
	for _, id := range want {
		ok := false
		for _, g := range got {
			if g == id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
