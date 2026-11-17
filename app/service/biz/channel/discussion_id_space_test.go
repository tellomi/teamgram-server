package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// repliesServiceUser is the @replies account official clients hard-code
// (contacts.blockFromReplies reads that private dialog, not this store).
const repliesServiceUser int64 = 1271266957

// TestDiscussionChainIDsStayOnTheChannel is the storage half of
// ISSUE-S-BLOCK-FROM-REPLIES-001. SetDiscussionGroup, the channel post,
// messages.getDiscussionMessage, a member message in the linked group, and
// a supergroup reply all live in the channel message store. None of those
// ids are written under the @replies user. The test does not change
// TELLOMI_CHANNEL_COMMENTS.
func TestDiscussionChainIDsStayOnTheChannel(t *testing.T) {
	svc, ctx := newTest(t)
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	channelChat := broadcast.Chats[0]
	group := megagroup(t, svc, ctx, "Talk")
	if err = svc.SetDiscussionGroup(ctx, 1, 0, inCh(channelChat), inCh(group)); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, inCh(group), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}

	postID := sendID(t, svc, ctx, 1, channelChat.Id, nil)
	disc, err := svc.DiscussionMessage(ctx, 1, channelChat.Id, postID)
	if err != nil {
		t.Fatal(err)
	}
	if len(disc.GetMessages()) == 0 {
		t.Fatal("discussion message has no messages")
	}
	commentID := sendID(t, svc, ctx, 2, group.Id, nil)

	plain := megagroup(t, svc, ctx, "Room")
	if _, err = svc.Invite(ctx, 1, 0, inCh(plain), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	rootID := sendID(t, svc, ctx, 1, plain.Id, nil)
	replyID := sendID(t, svc, ctx, 2, plain.Id, &mtproto.MessageReplyHeader{
		ReplyToMsgId:       rootID,
		ReplyToMsgId_INT32: rootID,
	})

	requireOnChannel(t, svc, ctx, channelChat.Id, postID, 1)
	requireOnChannel(t, svc, ctx, group.Id, commentID, 2)
	requireOnChannel(t, svc, ctx, plain.Id, rootID, 1)
	requireOnChannel(t, svc, ctx, plain.Id, replyID, 2)
	for _, m := range disc.GetMessages() {
		id := m.GetId()
		if id == 0 {
			t.Fatal("discussion message id is 0")
		}
		onNews, err := svc.store.GetMessage(ctx, channelChat.Id, id)
		if err != nil {
			t.Fatal(err)
		}
		onTalk, err := svc.store.GetMessage(ctx, group.Id, id)
		if err != nil {
			t.Fatal(err)
		}
		if onNews == nil && onTalk == nil {
			t.Fatalf("discussion id %d is not stored on the channel or the linked group", id)
		}
		absent, err := svc.store.GetMessage(ctx, repliesServiceUser, id)
		if err != nil || absent != nil {
			t.Fatalf("discussion id %d is stored for the replies user", id)
		}
	}
	for _, id := range []int32{postID, commentID, rootID, replyID} {
		absent, err := svc.store.GetMessage(ctx, repliesServiceUser, id)
		if err != nil || absent != nil {
			t.Fatalf("id %d is stored for the replies user", id)
		}
	}
	mem, ok := svc.store.(*memoryStore)
	if !ok {
		t.Fatal("test store is not the memory store")
	}
	allowed := map[int64]bool{channelChat.Id: true, group.Id: true, plain.Id: true}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	for id, rows := range mem.messages {
		if !allowed[id] {
			t.Fatalf("messages stored on %d", id)
		}
		for _, row := range rows {
			if row != nil && row.ChannelID != id {
				t.Fatalf("row channel %d key %d", row.ChannelID, id)
			}
		}
	}
	if _, ok := mem.messages[repliesServiceUser]; ok {
		t.Fatal("replies user has a channel message bucket")
	}
}

func sendID(t *testing.T, svc *Service, ctx context.Context, sender, channelID int64, reply *mtproto.MessageReplyHeader) int32 {
	t.Helper()
	msg := &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "m", Date: 10}
	if reply != nil {
		msg.ReplyTo = mtproto.MakeTLMessageReplyHeader(reply).To_MessageReplyHeader()
	}
	sent, err := svc.Send(ctx, sender, channelID, 0, []Outgoing{{RandomID: int64(channelID) + int64(sender), Message: msg}})
	if err != nil {
		t.Fatal(err)
	}
	if sent == nil {
		t.Fatal("send returned no updates")
	}
	for _, up := range sent.Updates {
		if m := up.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
			return m.GetId()
		}
	}
	t.Fatal("send updates have no message id")
	return 0
}

func requireOnChannel(t *testing.T, svc *Service, ctx context.Context, channelID int64, msgID int32, sender int64) {
	t.Helper()
	got, err := svc.store.GetMessage(ctx, channelID, msgID)
	if err != nil || got == nil || got.Deleted || got.ChannelID != channelID || got.ID != msgID || got.SenderID != sender {
		t.Fatalf("id %d channel %d sender %d missing (%v)", msgID, channelID, sender, err)
	}
}
