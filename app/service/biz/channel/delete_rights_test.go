package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestDeleteMessagesRights(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  &mtproto.Message{Message: "from creator", Date: 20},
	}})
	if err != nil {
		t.Fatal(err)
	}
	creatorMsg := sent.Updates[0].Message_MESSAGE.Id
	if _, _, err := svc.Delete(ctx, 2, ch.Id, 0, []int32{creatorMsg}); status.Convert(err).Message() != "MESSAGE_DELETE_FORBIDDEN" {
		t.Fatalf("member delete: %v", err)
	}
	if !historyHas(t, svc, ctx, 1, in, "from creator") {
		t.Fatal("creator message was deleted")
	}

	own, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 12,
		Message:  &mtproto.Message{Message: "from member", Date: 21},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ownID := own.Updates[0].Message_MESSAGE.Id
	if _, count, err := svc.Delete(ctx, 2, ch.Id, 0, []int32{ownID}); err != nil || count != 1 {
		t.Fatalf("own delete count=%d err=%v", count, err)
	}

	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{DeleteMessages: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	if _, count, err := svc.Delete(ctx, 3, ch.Id, 0, []int32{creatorMsg}); err != nil || count != 1 {
		t.Fatalf("admin delete count=%d err=%v", count, err)
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	nin := input(news.Chats[0].Id, news.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, nin, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	post, err := svc.Send(ctx, 1, news.Chats[0].Id, 0, []Outgoing{{
		RandomID: 13,
		Message:  &mtproto.Message{Message: "post", Date: 30},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Delete(ctx, 2, news.Chats[0].Id, 0, []int32{post.Updates[0].Message_MESSAGE.Id}); status.Convert(err).Message() != "MESSAGE_DELETE_FORBIDDEN" {
		t.Fatalf("subscriber delete: %v", err)
	}
	if _, _, err := svc.Delete(ctx, 2, ch.Id, 0, []int32{404}); err != nil {
		t.Fatalf("missing id: %v", err)
	}
}
