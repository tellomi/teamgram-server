package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestForumTopicCountAndUnread is rows 257 and 258.
// getForumTopics.count is the total, and each topic tracks unread until
// messages.readDiscussion.
func TestForumTopicCountAndUnread(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610704", "TopicOwner")
	bob, bobSelf := dialPhone(t, ctx, cfg, "9996610705", "TopicBob")
	cara, caraSelf := dialPhone(t, ctx, cfg, "9996610706", "TopicCara")

	id, hash, err := owner.CreateMegagroup(ctx, "topic room")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(id, hash)
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleForum{
		Constructor: mtproto.CRC32_channels_toggleForum_3ff75734,
		Channel:     channel,
		Enabled:     mtproto.BoolTrue,
		Tabs:        mtproto.BoolFalse,
	}); err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(id, hash)
	for i := 0; i < 8; i++ {
		if _, err = owner.Invoke(ctx, &mtproto.TLMessagesCreateForumTopic{
			Constructor: mtproto.CRC32_messages_createForumTopic,
			Peer:        peer,
			Title:       "Extra",
			RandomId:    randomID(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	page := forumTopics(t, owner, ctx, peer, 3, 0)
	if page.GetCount() != 9 || len(page.GetTopics()) != 3 {
		t.Fatalf("count %d page %d", page.GetCount(), len(page.GetTopics()))
	}

	if err = owner.InviteToChannel(ctx, channel, inputUser(bobSelf.ID, bobSelf.AccessHash), inputUser(caraSelf.ID, caraSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	var bobHash, caraHash int64
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		var ok bool
		bobHash, ok, err = bob.ChatAccess(ctx, id)
		if err != nil || !ok || bobHash == 0 {
			return false, err
		}
		caraHash, ok, err = cara.ChatAccess(ctx, id)
		return ok && caraHash != 0, err
	}); err != nil {
		t.Fatal(err)
	}
	all := forumTopics(t, owner, ctx, peer, 20, 0)
	alpha := forumByTitle(t, all.GetTopics(), "Extra")
	beta := secondTitle(t, all.GetTopics(), "Extra", alpha.GetId())
	a1 := sendTopic(t, bob, ctx, inputPeerChannel(id, bobHash), alpha.GetId(), "a1")
	a2 := sendTopic(t, bob, ctx, inputPeerChannel(id, bobHash), alpha.GetId(), "a2")
	sendTopic(t, cara, ctx, inputPeerChannel(id, caraHash), beta.GetId(), "b1")

	seen := forumTopics(t, owner, ctx, peer, 20, 0)
	if got := forumByID(t, seen.GetTopics(), alpha.GetId()); got.GetUnreadCount() < 2 {
		t.Fatalf("alpha unread %d", got.GetUnreadCount())
	}
	if got := forumByID(t, seen.GetTopics(), beta.GetId()); got.GetUnreadCount() < 1 {
		t.Fatalf("beta unread %d", got.GetUnreadCount())
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesReadDiscussion{
		Constructor: mtproto.CRC32_messages_readDiscussion,
		Peer:        peer,
		MsgId:       alpha.GetId(),
		ReadMaxId:   a2,
	}); err != nil {
		t.Fatal(err)
	}
	_ = a1
	after := forumTopics(t, owner, ctx, peer, 20, 0)
	if got := forumByID(t, after.GetTopics(), alpha.GetId()); got.GetUnreadCount() != 0 || got.GetReadInboxMaxId() < a2 {
		t.Fatalf("alpha after read unread=%d inbox=%d", got.GetUnreadCount(), got.GetReadInboxMaxId())
	}
	if got := forumByID(t, after.GetTopics(), beta.GetId()); got.GetUnreadCount() < 1 {
		t.Fatalf("beta after read unread %d", got.GetUnreadCount())
	}
}

func forumTopics(t *testing.T, c *Client, ctx context.Context, peer *mtproto.InputPeer, limit, offset int32) *mtproto.Messages_ForumTopics {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetForumTopics{
		Constructor: mtproto.CRC32_messages_getForumTopics,
		Peer:        peer,
		Limit:       limit,
		OffsetTopic: offset,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesForumTopics:
		return v.To_Messages_ForumTopics()
	case *mtproto.Messages_ForumTopics:
		return v
	default:
		t.Fatalf("topics %T", obj)
		return nil
	}
}

func sendTopic(t *testing.T, c *Client, ctx context.Context, peer *mtproto.InputPeer, topicID int32, text string) int32 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     text,
		RandomId:    randomID(),
		ReplyTo: mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
			ReplyToMsgId: topicID,
			TopMsgId:     wrapperspb.Int32(topicID),
		}).To_InputReplyTo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range messagesOfUpdates(obj) {
		if m.GetMessage() == text {
			return m.GetId()
		}
	}
	t.Fatalf("sent %q not in %T", text, obj)
	return 0
}

func messagesOfUpdates(obj mtproto.TLObject) []*mtproto.Message {
	up, ok := obj.(*mtproto.Updates)
	if !ok {
		if box, ok := obj.(*mtproto.TLUpdates); ok {
			up = box.To_Updates()
		}
	}
	if up == nil {
		return nil
	}
	var out []*mtproto.Message
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m != nil {
			out = append(out, m)
		}
	}
	return out
}

func forumByTitle(t *testing.T, topics []*mtproto.ForumTopic, title string) *mtproto.ForumTopic {
	t.Helper()
	for _, topic := range topics {
		if topic.GetTitle() == title {
			return topic
		}
	}
	t.Fatalf("title %q missing", title)
	return nil
}

func secondTitle(t *testing.T, topics []*mtproto.ForumTopic, title string, skip int32) *mtproto.ForumTopic {
	t.Helper()
	for _, topic := range topics {
		if topic.GetTitle() == title && topic.GetId() != skip {
			return topic
		}
	}
	t.Fatalf("second %q missing", title)
	return nil
}

func forumByID(t *testing.T, topics []*mtproto.ForumTopic, id int32) *mtproto.ForumTopic {
	t.Helper()
	for _, topic := range topics {
		if topic.GetId() == id {
			return topic
		}
	}
	t.Fatalf("topic %d missing", id)
	return nil
}
