package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestUnreadReactionsTTLAndPolicy(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "React")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 11, Message: &mtproto.Message{Message: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}
	unread, err := svc.UnreadReactions(ctx, 1, ch.Id, 0, 0, 0, 20, 0, 0)
	if err != nil || len(unread.Messages) != 1 || unread.Messages[0].GetReactions().GetResults()[0].GetCount() != 1 {
		t.Fatalf("unread %+v %v", unread, err)
	}
	own, err := svc.UnreadReactions(ctx, 2, ch.Id, 0, 0, 0, 20, 0, 0)
	if err != nil || len(own.Messages) != 0 {
		t.Fatalf("sender saw own reaction %+v %v", own, err)
	}
	if _, err := svc.ReadReactions(ctx, 1, ch.Id, 0); err != nil {
		t.Fatal(err)
	}
	unread, err = svc.UnreadReactions(ctx, 1, ch.Id, 0, 0, 0, 20, 0, 0)
	if err != nil || len(unread.Messages) != 0 {
		t.Fatalf("still unread %+v %v", unread, err)
	}
	if _, err := svc.SetAvailableReactions(ctx, 1, ch.Id, "none", false, nil, nil, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"❤"}, false); err != mtproto.ErrReactionInvalid {
		t.Fatalf("none policy: %v", err)
	}
	if _, err := svc.DeleteParticipantReaction(ctx, 1, ch.Id, msgID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetHistoryTTL(ctx, 1, ch.Id, 86400); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetHistoryTTL(ctx, 1, ch.Id, -1); err != mtproto.ErrTtlPeriodInvalid {
		t.Fatal("negative ttl accepted")
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.FullChat.GetTtlPeriod().GetValue() != 86400 {
		t.Fatalf("ttl %+v %v", full.GetFullChat().GetTtlPeriod(), err)
	}
	if full.FullChat.GetAvailableReactions_FLAGCHATREACTIONS().GetPredicateName() != mtproto.Predicate_chatReactionsNone {
		t.Fatalf("reactions %#v", full.FullChat.GetAvailableReactions_FLAGCHATREACTIONS())
	}
	if _, err := svc.SetHistoryTTL(ctx, 2, ch.Id, 0); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member ttl: %v", err)
	}
}
