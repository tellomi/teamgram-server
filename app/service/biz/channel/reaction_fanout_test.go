package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMegagroupReactionPushesAndUnreadBadge(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "ReactPush")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	var pushed []int64
	var badges []int32
	svc.SetHooks(Hooks{
		Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
			if updates == nil {
				return
			}
			for _, u := range updates.Updates {
				if u.GetPredicateName() == mtproto.Predicate_updateMessageReactions {
					pushed = append(pushed, userID)
				}
			}
		},
		ReactionBadge: func(_ context.Context, userID, _ int64, delta int32) {
			if userID == 1 {
				badges = append(badges, delta)
			}
		},
	})
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "hi", Date: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	pushed = nil
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"👍"}, false); err != nil {
		t.Fatal(err)
	}
	if !hasID(pushed, 1) || !hasID(pushed, 2) {
		t.Fatalf("pushed %v", pushed)
	}
	if len(badges) != 1 || badges[0] != 1 {
		t.Fatalf("badge after react %v", badges)
	}
	if _, err := svc.ReadReactions(ctx, 1, ch.Id, 0); err != nil {
		t.Fatal(err)
	}
	if len(badges) != 2 || badges[1] != -1 {
		t.Fatalf("badge after read %v", badges)
	}
	if _, err := svc.React(ctx, 2, ch.Id, msgID, []string{"❤️"}, false); err != nil {
		t.Fatal(err)
	}
	pts, count, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{msgID})
	if err != nil || pts == 0 || count == 0 {
		t.Fatalf("delete pts=%d count=%d err=%v", pts, count, err)
	}
	if badges[len(badges)-1] != -1 {
		t.Fatalf("badge after delete %v", badges)
	}
}

func hasID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
