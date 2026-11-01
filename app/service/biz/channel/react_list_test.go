package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMegagroupReactionListAndBroadcastForbidden(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Reactors")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 5, Message: &mtproto.Message{Message: "hi", Date: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	if _, err := svc.React(ctx, 1, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ReactionList(ctx, 1, ch.Id, 0, msgID, nil, "", 1)
	if err != nil || list.Count != 2 || list.GetNextOffset().GetValue() != "1" || len(list.Reactions) != 1 {
		t.Fatalf("list %#v %v", list, err)
	}
	if list.Reactions[0].GetPeerId().GetUserId() != 2 {
		t.Fatalf("newest %#v", list.Reactions[0])
	}
	again, err := svc.ReactionsFor(ctx, 1, ch.Id, []int32{msgID, 0})
	if err != nil {
		t.Fatal(err)
	}
	rx := again.Updates[0].GetReactions_MESSAGEREACTIONS()
	if !rx.GetCanSeeList() || len(rx.GetRecentReactions()) != 2 || rx.GetResults()[0].Count != 2 {
		t.Fatalf("refresh %#v", rx)
	}
	if _, err := svc.ReactionList(ctx, 1, ch.Id, 0, 999, nil, "", 10); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("missing: %v", err)
	}
	if _, err := svc.ReactionsFor(ctx, 1, ch.Id, []int32{999}); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("refresh missing: %v", err)
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	post, err := svc.Send(ctx, 1, news.Chats[0].Id, 0, []Outgoing{{RandomID: 8, Message: &mtproto.Message{Message: "post", Date: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	postID := post.Updates[0].Message_MESSAGE.Id
	if _, err := svc.React(ctx, 1, news.Chats[0].Id, postID, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReactionList(ctx, 1, news.Chats[0].Id, 0, postID, nil, "", 10); err != mtproto.ErrBroadcastForbidden {
		t.Fatalf("broadcast list: %v", err)
	}
	counts, err := svc.ReactionsFor(ctx, 1, news.Chats[0].Id, []int32{postID})
	if err != nil {
		t.Fatal(err)
	}
	hidden := counts.Updates[0].GetReactions_MESSAGEREACTIONS()
	if hidden.GetCanSeeList() || len(hidden.GetRecentReactions()) != 0 || hidden.GetResults()[0].Count != 1 {
		t.Fatalf("broadcast counts %#v", hidden)
	}
}
