package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestForumTopicCountIsTotal(t *testing.T) {
	svc, ctx := newTest(t)
	ch := forumChannel(t, svc, ctx)
	for i := 0; i < 8; i++ {
		if _, _, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "T", 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	page, _, total, err := svc.ListTopics(ctx, 1, ch.Id, "", 3, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 9 || len(page) != 3 {
		t.Fatalf("page %d total %d", len(page), total)
	}
	next, _, total2, err := svc.ListTopics(ctx, 1, ch.Id, "", 3, 0, 0, page[len(page)-1].GetId())
	if err != nil {
		t.Fatal(err)
	}
	if total2 != 9 || len(next) != 3 {
		t.Fatalf("next page %d total %d", len(next), total2)
	}
}

func TestForumTopicUnreadFollowsReadDiscussion(t *testing.T) {
	svc, ctx := newTest(t)
	ch := forumChannel(t, svc, ctx)
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	_, topicA, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Alpha", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, topicB, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Beta", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	a1 := sendInTopic(t, svc, ctx, 2, ch.Id, topicA.ID, "a1")
	a2 := sendInTopic(t, svc, ctx, 2, ch.Id, topicA.ID, "a2")
	sendInTopic(t, svc, ctx, 3, ch.Id, topicB.ID, "b1")

	topics, _, _, err := svc.ListTopics(ctx, 1, ch.Id, "", 20, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := topicByID(t, topics, topicA.ID); got.GetUnreadCount() != 2 || got.GetReadInboxMaxId() != 0 {
		t.Fatalf("alpha before read unread=%d inbox=%d", got.GetUnreadCount(), got.GetReadInboxMaxId())
	}
	if got := topicByID(t, topics, topicB.ID); got.GetUnreadCount() != 1 {
		t.Fatalf("beta before read unread=%d", got.GetUnreadCount())
	}
	if err = svc.ReadTopic(ctx, 1, ch.Id, topicA.ID, a2); err != nil {
		t.Fatal(err)
	}
	topics, _, _, err = svc.ListTopics(ctx, 1, ch.Id, "", 20, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := topicByID(t, topics, topicA.ID); got.GetUnreadCount() != 0 || got.GetReadInboxMaxId() != a2 {
		t.Fatalf("alpha after read unread=%d inbox=%d want inbox %d", got.GetUnreadCount(), got.GetReadInboxMaxId(), a2)
	}
	if got := topicByID(t, topics, topicB.ID); got.GetUnreadCount() != 1 || got.GetReadInboxMaxId() != 0 {
		t.Fatalf("beta after read unread=%d inbox=%d", got.GetUnreadCount(), got.GetReadInboxMaxId())
	}
	if a1 > a2 {
		t.Fatalf("ids %d %d", a1, a2)
	}
}

func forumChannel(t *testing.T, svc *Service, ctx context.Context) *mtproto.Chat {
	t.Helper()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	return created.Chats[0]
}

func sendInTopic(t *testing.T, svc *Service, ctx context.Context, from, channelID int64, topicID int32, text string) int32 {
	t.Helper()
	reply := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: topicID,
		ReplyToTopId: &wrapperspb.Int32Value{Value: topicID},
		ForumTopic:   true,
	}).To_MessageReplyHeader()
	var random int64
	for _, b := range text {
		random = random*131 + int64(b)
	}
	random = random*1000 + int64(topicID)
	up, err := svc.Send(ctx, from, channelID, 0, []Outgoing{{
		RandomID: random,
		Message:  &mtproto.Message{Message: text, ReplyTo: reply},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return up.Updates[0].GetMessage_MESSAGE().GetId()
}

func topicByID(t *testing.T, topics []*mtproto.ForumTopic, id int32) *mtproto.ForumTopic {
	t.Helper()
	for _, topic := range topics {
		if topic.GetId() == id {
			return topic
		}
	}
	t.Fatalf("topic %d missing", id)
	return nil
}
