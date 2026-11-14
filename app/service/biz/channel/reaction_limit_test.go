package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestReactionLimitCountsOtherMembers(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "React")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "hi", Date: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := firstMessageID(sent)
	if msgID == 0 {
		t.Fatal("no message id")
	}
	if _, err := svc.SetAvailableReactions(ctx, 1, ch.Id, "all", false, nil, nil, 1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, 1, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"❤️"}, false); status.Convert(err).Message() != "REACTIONS_TOO_MANY" {
		t.Fatalf("second emoji: %v", err)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatalf("same emoji: %v", err)
	}
}

func firstMessageID(up *mtproto.Updates) int32 {
	if up == nil {
		return 0
	}
	for _, u := range up.Updates {
		if u.GetMessage_MESSAGE() != nil && u.GetMessage_MESSAGE().GetId() != 0 {
			return u.GetMessage_MESSAGE().GetId()
		}
	}
	return 0
}
