package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestBlockFromRepliesChannelMessage(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "")
	prevCh := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prevCh) })
	prevBook := statusbook.Shared()
	statusbook.Install(statusbook.New(nil))
	t.Cleanup(func() { statusbook.Install(prevBook) })

	ctx := userCtx(3)
	const bob int64 = 4
	group, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 3, Title: "Replies", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	chat := group.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  chat.Id,
		AccessHash: chat.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	if _, err := svc.Invite(ctx, 3, 0, in, []int64{bob}, nil); err != nil {
		t.Fatal(err)
	}
	root, err := svc.Send(ctx, 3, chat.Id, 0, []channel.Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "root"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var rootID int32
	for _, u := range root.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m.GetMessage() == "root" {
			rootID = m.GetId()
		}
	}
	reply, err := svc.Send(ctx, bob, chat.Id, 0, []channel.Outgoing{{
		RandomID: 2,
		Message: &mtproto.Message{
			PredicateName: mtproto.Predicate_message,
			Message:       "answer",
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:       rootID,
				ReplyToMsgId_INT32: rootID,
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var replyID int32
	for _, u := range reply.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m.GetMessage() == "answer" {
			replyID = m.GetId()
		}
	}
	if replyID == 0 {
		t.Fatal("reply id missing")
	}

	var blocked int64
	var synced bool
	s := ThreadsService{
		Messages: repliesMsgFake{},
		Users:    repliesUserFake{blocked: &blocked},
		Sync:     repliesSyncFake{called: &synced},
	}
	up, err := s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{MsgId: replyID})
	if err != nil {
		t.Fatal(err)
	}
	if blocked != bob || !synced {
		t.Fatalf("blocked %d synced %v", blocked, synced)
	}
	if up == nil || len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPeerId().GetUserId() != bob {
		t.Fatalf("updates %#v", up)
	}
	if _, err := s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{MsgId: replyID + 80}); status.Convert(err).Message() != "MSG_ID_INVALID" {
		t.Fatalf("unknown id %v", err)
	}
	// The same numeric id is already a private message. It is not in @replies.
	s.Messages = repliesMsgFake{box: &mtproto.MessageBox{
		UserId:    3,
		MessageId: replyID,
		PeerType:  mtproto.PEER_USER,
		PeerId:    99,
		Message:   &mtproto.Message{Message: "private"},
	}}
	blocked = 0
	if _, err := s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{MsgId: replyID}); err != nil || blocked != bob {
		t.Fatalf("private id collision blocked %d %v", blocked, err)
	}

	news, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 3, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	talk, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 3, Title: "Talk", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	talkChat := talk.Chats[0]
	if err := svc.SetDiscussionGroup(ctx, 3, 0,
		mtproto.MakeTLInputChannel(&mtproto.InputChannel{
			ChannelId:  news.Chats[0].Id,
			AccessHash: news.Chats[0].GetAccessHash_FLAGINT64().GetValue(),
		}).To_InputChannel(),
		mtproto.MakeTLInputChannel(&mtproto.InputChannel{
			ChannelId:  talkChat.Id,
			AccessHash: talkChat.GetAccessHash_FLAGINT64().GetValue(),
		}).To_InputChannel(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, 3, 0, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  talkChat.Id,
		AccessHash: talkChat.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel(), []int64{bob}, nil); err != nil {
		t.Fatal(err)
	}
	comment, err := svc.Send(ctx, bob, talkChat.Id, 0, []channel.Outgoing{{
		RandomID: 3,
		Message:  &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "comment"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var commentID int32
	for _, u := range comment.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m.GetMessage() == "comment" {
			commentID = m.GetId()
		}
	}
	blocked = 0
	up, err = s.ContactsBlockFromReplies(ctx, &mtproto.TLContactsBlockFromReplies{
		DeleteMessage: false,
		DeleteHistory: false,
		ReportSpam:    false,
		MsgId:         commentID,
	})
	if err != nil || blocked != bob || up.GetUpdates()[0].GetPeerId().GetUserId() != bob {
		t.Fatalf("discussion blocked %d %#v %v", blocked, up, err)
	}
}

func userCtx(id int64) context.Context {
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: id, PermAuthKeyId: 9})
	if err != nil {
		panic(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}
