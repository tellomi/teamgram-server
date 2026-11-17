package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

// ISSUE-S-BLOCK-FROM-REPLIES-001. Production called contacts.blockFromReplies
// with the supergroup reply id and with the discussion-group comment id.
// CHANNEL_COMMENTS stayed off. Both ids exist; MSG_ID_INVALID is wrong.
func TestReplyAuthorForBlockSupergroupAndDiscussion(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "")
	if commentsOn() {
		t.Fatal("CHANNEL_COMMENTS must stay off")
	}
	svc, ctx := newTest(t)
	const (
		alice = int64(1)
		bob   = int64(2)
		cara  = int64(3)
	)

	group := megagroup(t, svc, ctx, "Replies")
	gin := inCh(group)
	if _, err := svc.Invite(ctx, alice, 0, gin, []int64{bob}, nil); err != nil {
		t.Fatal(err)
	}
	root, err := svc.Send(ctx, alice, group.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  textMsg("root"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	rootID := sentMessageID(t, root)
	reply, err := svc.Send(ctx, bob, group.Id, 0, []Outgoing{{
		RandomID: 12,
		Message:  replyMsg("answer", rootID),
	}})
	if err != nil {
		t.Fatal(err)
	}
	replyID := sentMessageID(t, reply)
	got, err := svc.ReplyAuthorForBlock(ctx, alice, replyID)
	if err != nil || got != bob {
		t.Fatalf("supergroup reply author %d %v", got, err)
	}
	if _, err := svc.ReplyAuthorForBlock(ctx, alice, replyID+50); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("missing id %v", err)
	}
	plain, err := svc.Send(ctx, bob, group.Id, 0, []Outgoing{{
		RandomID: 13,
		Message:  textMsg("not a reply"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReplyAuthorForBlock(ctx, alice, sentMessageID(t, plain)); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("plain megagroup message %v", err)
	}
	if _, err := svc.Leave(ctx, bob, 0, gin); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReplyAuthorForBlock(ctx, bob, replyID); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("left member still resolved the reply %v", err)
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: alice, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	talk := megagroup(t, svc, ctx, "Talk")
	if err := svc.SetDiscussionGroup(ctx, alice, 0, inCh(news.Chats[0]), inCh(talk)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, alice, 0, inCh(talk), []int64{bob}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, alice, news.Chats[0].Id, 0, []Outgoing{{
		RandomID: 21,
		Message:  textMsg("post"),
	}}); err != nil {
		t.Fatal(err)
	}
	// The comment is an ordinary megagroup send. The comments flag is off,
	// so nothing is mirrored and the message has no reply header.
	comment, err := svc.Send(ctx, bob, talk.Id, 0, []Outgoing{{
		RandomID: 22,
		Message:  textMsg("comment"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	commentID := sentMessageID(t, comment)
	got, err = svc.ReplyAuthorForBlock(ctx, alice, commentID)
	if err != nil || got != bob {
		t.Fatalf("discussion comment author %d %v", got, err)
	}

	other := megagroup(t, svc, ctx, "Other")
	if _, err := svc.Invite(ctx, alice, 0, inCh(other), []int64{cara}, nil); err != nil {
		t.Fatal(err)
	}
	otherRoot, err := svc.Send(ctx, alice, other.Id, 0, []Outgoing{{
		RandomID: 31,
		Message:  textMsg("other root"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Same numeric id as Bob's reply, but Cara answered Alice too.
	// Two authors at the same rank is not a guess.
	forced, err := svc.Send(ctx, cara, other.Id, 0, []Outgoing{{
		RandomID: 32,
		Message:  replyMsg("other answer", sentMessageID(t, otherRoot)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sentMessageID(t, forced), replyID; got != want {
		t.Fatalf("collision setup reply %d other %d", want, got)
	}
	if _, err := svc.ReplyAuthorForBlock(ctx, alice, replyID); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("ambiguous reply id %v", err)
	}
}

func textMsg(text string) *mtproto.Message {
	return &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: text}
}

func replyMsg(text string, to int32) *mtproto.Message {
	msg := textMsg(text)
	msg.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:       to,
		ReplyToMsgId_INT32: to,
	}).To_MessageReplyHeader()
	return msg
}

func sentMessageID(t *testing.T, up *mtproto.Updates) int32 {
	t.Helper()
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m.GetMessage() != "" && m.GetId() != 0 {
			return m.GetId()
		}
	}
	t.Fatalf("no message id in %#v", up.GetUpdates())
	return 0
}
