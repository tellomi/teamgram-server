package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBroadcastViewsCountOncePerUser(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 4, Message: &mtproto.Message{Message: "post", Date: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()

	first, err := svc.MessageViews(ctx, 2, ch.Id, []int32{msgID, 999}, true)
	if err != nil || len(first) != 2 || first[0].GetViews().GetValue() != 2 || first[1].GetViews() != nil {
		t.Fatalf("first views %+v %v", first, err)
	}
	again, err := svc.MessageViews(ctx, 2, ch.Id, []int32{msgID}, true)
	if err != nil || again[0].GetViews().GetValue() != 2 {
		t.Fatalf("second view counted twice: %+v %v", again, err)
	}
	third, err := svc.MessageViews(ctx, 3, ch.Id, []int32{msgID}, true)
	if err != nil || third[0].GetViews().GetValue() != 3 {
		t.Fatalf("member 3: %+v %v", third, err)
	}
	author, err := svc.MessageViews(ctx, 1, ch.Id, []int32{msgID}, true)
	if err != nil || author[0].GetViews().GetValue() != 3 {
		t.Fatalf("author was counted twice: %+v %v", author, err)
	}
	if _, err := svc.MessageViews(ctx, 99, ch.Id, []int32{msgID}, true); err != mtproto.ErrChannelPrivate {
		t.Fatalf("outsider: %v", err)
	}
}

func TestSupergroupReadParticipantsMentionsAndEditData(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	photo := &mtproto.Message{
		Message: "cap",
		Date:    20,
		Media:   mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia(),
	}
	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 8, Message: photo}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()

	caption, err := svc.MessageEditData(ctx, 2, ch.Id, msgID)
	if err != nil || !caption {
		t.Fatalf("author caption %v %v", caption, err)
	}
	if _, err := svc.MessageEditData(ctx, 3, ch.Id, msgID); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("other member: %v", err)
	}
	if caption, err := svc.MessageEditData(ctx, 1, ch.Id, msgID); err != nil || !caption {
		t.Fatalf("creator edit data %v %v", caption, err)
	}
	plain, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 9, Message: &mtproto.Message{Message: "text", Date: 21}}})
	if err != nil {
		t.Fatal(err)
	}
	plainID := plain.Updates[0].GetMessage_MESSAGE().GetId()
	caption, err = svc.MessageEditData(ctx, 2, ch.Id, plainID)
	if err != nil || caption {
		t.Fatalf("text caption %v %v", caption, err)
	}

	mention := &mtproto.Message{
		Message: "hey",
		Date:    22,
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
				Offset: 0, Length: 3, UserId_INT64: 1,
			}).To_MessageEntity(),
		},
	}
	mentioned, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 10, Message: mention}})
	if err != nil {
		t.Fatal(err)
	}
	mentionID := mentioned.Updates[0].GetMessage_MESSAGE().GetId()
	reply, err := svc.Send(ctx, 3, ch.Id, 0, []Outgoing{{RandomID: 11, Message: &mtproto.Message{
		Message: "re",
		Date:    23,
		ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: plainID, ReplyToMsgId_INT32: plainID}).To_MessageReplyHeader(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	replyID := reply.Updates[0].GetMessage_MESSAGE().GetId()

	unread, err := svc.UnreadMentions(ctx, 1, ch.Id, 0, 0, 20, 0, 0)
	if err != nil || unread.GetCount() != 1 || unread.Messages[0].GetId() != mentionID {
		t.Fatalf("user 1 mentions %+v %v", unread, err)
	}
	replies, err := svc.UnreadMentions(ctx, 2, ch.Id, 0, 0, 20, 0, 0)
	if err != nil || replies.GetCount() != 1 || replies.Messages[0].GetId() != replyID {
		t.Fatalf("reply mention %+v %v", replies, err)
	}
	if err := svc.ReadHistory(ctx, 1, in, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReadHistory(ctx, 3, in, 0); err != nil {
		t.Fatal(err)
	}
	unread, err = svc.UnreadMentions(ctx, 1, ch.Id, 0, 0, 20, 0, 0)
	if err != nil || unread.GetCount() != 0 {
		t.Fatalf("mentions stayed unread %+v %v", unread, err)
	}

	readers, err := svc.ReadParticipants(ctx, 2, ch.Id, msgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(readers) != 2 || readers[0].UserID != 1 || readers[1].UserID != 3 {
		t.Fatalf("readers %+v", readers)
	}
	if readers[0].Date == 0 || readers[1].Date == 0 {
		t.Fatalf("read dates %+v", readers)
	}
	self, err := svc.ReadParticipants(ctx, 1, ch.Id, msgID)
	if err != nil || len(self) != 1 || self[0].UserID != 3 {
		t.Fatalf("caller excluded %+v %v", self, err)
	}

	views, err := svc.MessageViews(ctx, 1, ch.Id, []int32{msgID}, true)
	if err != nil || len(views) != 1 || views[0].GetViews() != nil {
		t.Fatalf("megagroup message has no view counter: %+v %v", views, err)
	}
}

func TestForwardedPostViewsFollowTheOriginal(t *testing.T) {
	svc, ctx := newTest(t)
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true, About: "pub"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, inCh(news.Chats[0]), "news_one"); err != nil {
		t.Fatal(err)
	}
	post, err := svc.Send(ctx, 1, news.Chats[0].Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "orig", Date: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	postID := post.Updates[0].GetMessage_MESSAGE().GetId()

	room := megagroup(t, svc, ctx, "Talk")
	in := inCh(room)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{4}, nil); err != nil {
		t.Fatal(err)
	}
	fwd := &mtproto.Message{
		Message: "orig",
		Date:    6,
		FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
			FromId:      mtproto.MakePeerChannel(news.Chats[0].Id),
			ChannelPost: wrapperspb.Int32(postID),
			Date:        5,
		}).To_MessageFwdHeader(),
	}
	sent, err := svc.Send(ctx, 1, room.Id, 0, []Outgoing{{RandomID: 12, Message: fwd}})
	if err != nil {
		t.Fatal(err)
	}
	localID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	got, err := svc.MessageViews(ctx, 4, room.Id, []int32{localID}, true)
	if err != nil || got[0].GetViews().GetValue() != 2 {
		t.Fatalf("forward views %+v %v", got, err)
	}
	again, err := svc.MessageViews(ctx, 4, room.Id, []int32{localID}, true)
	if err != nil || again[0].GetViews().GetValue() != 2 {
		t.Fatalf("forward counted twice %+v %v", again, err)
	}
}
