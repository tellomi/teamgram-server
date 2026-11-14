package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestChannelCommentsMirrorAndGuestReply(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	svc, ctx := newTest(t)
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	channelChat := broadcast.Chats[0]
	channelIn := inCh(channelChat)
	if _, err := svc.UpdateUsername(ctx, 1, 0, channelIn, "newsch"); err != nil {
		t.Fatal(err)
	}
	group := megagroup(t, svc, ctx, "Talk")
	groupIn := inCh(group)
	if err := svc.SetDiscussionGroup(ctx, 1, channelIn, groupIn); err != nil {
		t.Fatal(err)
	}

	sent, err := svc.Send(ctx, 1, channelChat.Id, 0, []Outgoing{{
		RandomID: 1,
		Message: &mtproto.Message{
			PredicateName: mtproto.Predicate_message,
			Message:       "hello from the channel",
			Date:          10,
		},
	}})
	if err != nil || len(sent.Updates) == 0 || sent.Updates[0].GetMessage_MESSAGE() == nil {
		t.Fatalf("post %#v %v", sent, err)
	}
	post := sent.Updates[0].GetMessage_MESSAGE()
	if err := post.Encode(mtproto.NewEncodeBuf(4096), 232); err != nil {
		t.Fatal(err)
	}
	replies := post.GetReplies()
	if replies == nil || !replies.GetComments() || replies.GetChannelId().GetValue() != group.Id || replies.GetReplies() != 0 {
		t.Fatalf("replies on post %#v", replies)
	}

	disc, err := svc.DiscussionMessage(ctx, 1, channelChat.Id, post.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if err := disc.Encode(mtproto.NewEncodeBuf(8192), 232); err != nil {
		t.Fatal(err)
	}
	root := forwardedPost(disc.GetMessages(), channelChat.Id, post.GetId())
	if root == nil {
		t.Fatalf("discussion has no forward %#v", disc.GetMessages())
	}
	if root.GetFromId().GetChannelId() != channelChat.Id {
		t.Fatalf("forward from %#v", root.GetFromId())
	}

	guest := int64(9)
	comment, err := svc.Send(ctx, guest, group.Id, 0, []Outgoing{{
		RandomID: 2,
		Message: &mtproto.Message{
			PredicateName: mtproto.Predicate_message,
			Message:       "nice post",
			Date:          11,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:       root.GetId(),
				ReplyToMsgId_INT32: root.GetId(),
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatalf("guest comment: %v", err)
	}
	if _, err := svc.GetParticipant(ctx, 1, groupIn, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: guest}).To_InputPeer()); status.Convert(err).Message() != "USER_NOT_PARTICIPANT" {
		t.Fatalf("guest was added to the group: %v", err)
	}
	parts, err := svc.GetParticipants(ctx, 1, groupIn, nil, 0, 50, 0)
	if err != nil || parts.Count != 1 {
		t.Fatalf("group members %d %v", parts.Count, err)
	}

	again, err := svc.Send(ctx, 8, group.Id, 0, []Outgoing{{
		RandomID: 3,
		Message: &mtproto.Message{
			PredicateName: mtproto.Predicate_message,
			Message:       "also nice",
			Date:          12,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:       root.GetId(),
				ReplyToMsgId_INT32: root.GetId(),
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// A viewer can also reply to the channel post itself. The server places
	// that in the discussion thread and does not publish a new channel post.
	if _, err := svc.Send(ctx, 7, channelChat.Id, 0, []Outgoing{{
		RandomID: 5,
		Message: &mtproto.Message{
			PredicateName: mtproto.Predicate_message,
			Message:       "from the post",
			Date:          13,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:       post.GetId(),
				ReplyToMsgId_INT32: post.GetId(),
			}).To_MessageReplyHeader(),
		},
	}}); err != nil {
		t.Fatalf("reply on the channel post: %v", err)
	}
	if _, err := svc.GetParticipant(ctx, 1, channelIn, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7}).To_InputPeer()); status.Convert(err).Message() != "USER_NOT_PARTICIPANT" {
		t.Fatalf("channel reply added the commenter: %v", err)
	}
	_ = comment
	_ = again

	got, err := svc.GetMessages(ctx, 1, channelIn, []int32{post.GetId()})
	if err != nil || len(got.Messages) != 1 {
		t.Fatalf("get post %#v %v", got, err)
	}
	replies = got.Messages[0].GetReplies()
	if replies.GetReplies() != 3 || replies.GetMaxId().GetValue() == 0 || replies.GetChannelId().GetValue() != group.Id {
		t.Fatalf("updated replies %#v", replies)
	}
	if len(replies.GetRecentRepliers()) != 3 || replies.GetRecentRepliers()[0].GetUserId() != 7 || replies.GetRecentRepliers()[1].GetUserId() != 8 {
		t.Fatalf("recent repliers %#v", replies.GetRecentRepliers())
	}

	thread, err := svc.Replies(ctx, guest, channelChat.Id, post.GetId(), HistoryQuery{Limit: 10})
	if err != nil || thread.Count != 3 || len(thread.Messages) != 3 {
		t.Fatalf("getReplies count=%d len=%d %v", thread.GetCount(), len(thread.GetMessages()), err)
	}
	fromGroup, err := svc.Replies(ctx, guest, group.Id, root.GetId(), HistoryQuery{Limit: 10})
	if err != nil || fromGroup.Count != 3 {
		t.Fatalf("getReplies on the root %#v %v", fromGroup, err)
	}
	seen, err := svc.DiscussionMessage(ctx, guest, group.Id, root.GetId())
	if err != nil || forwardedPost(seen.GetMessages(), channelChat.Id, post.GetId()) == nil {
		t.Fatalf("guest discussion %#v %v", seen, err)
	}
	var leftGroup bool
	for _, chat := range seen.GetChats() {
		if chat.GetId() == group.Id && chat.GetMegagroup() && chat.GetLeft() && chat.GetAccessHash_FLAGINT64().GetValue() != 0 {
			leftGroup = true
		}
	}
	if !leftGroup {
		t.Fatalf("guest did not receive the discussion group %#v", seen.GetChats())
	}

	if _, err := svc.Send(ctx, guest, group.Id, 0, []Outgoing{{
		RandomID: 4,
		Message:  &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "not a comment", Date: 13},
	}}); status.Convert(err).Message() != "CHANNEL_PRIVATE" {
		t.Fatalf("plain guest send: %v", err)
	}
}

func TestChannelCommentsDisabledByDefault(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "")
	svc, ctx := newTest(t)
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	channelChat := broadcast.Chats[0]
	channelIn := inCh(channelChat)
	if _, err := svc.UpdateUsername(ctx, 1, 0, channelIn, "newsoff"); err != nil {
		t.Fatal(err)
	}
	group := megagroup(t, svc, ctx, "Talk off")
	if err := svc.SetDiscussionGroup(ctx, 1, channelIn, inCh(group)); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, channelChat.Id, 0, []Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "plain", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	post := sent.Updates[0].GetMessage_MESSAGE()
	if post.GetReplies() != nil {
		t.Fatalf("flag off still set replies %#v", post.GetReplies())
	}
	if _, err := svc.Send(ctx, 9, group.Id, 0, []Outgoing{{
		RandomID: 2,
		Message:  &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "nope", Date: 11},
	}}); status.Convert(err).Message() != "CHANNEL_PRIVATE" {
		t.Fatalf("flag off guest send: %v", err)
	}
}

func forwardedPost(msgs []*mtproto.Message, channelID int64, postID int32) *mtproto.Message {
	for _, m := range msgs {
		fwd := m.GetFwdFrom()
		if fwd == nil || fwd.GetChannelPost().GetValue() != postID {
			continue
		}
		if fwd.GetSavedFromPeer().GetChannelId() != channelID || fwd.GetSavedFromMsgId().GetValue() != postID {
			continue
		}
		return m
	}
	return nil
}
