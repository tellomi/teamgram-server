package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSetHistoryTTLInsertsServiceMessage(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "TTL")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}

	var pushed []pushedUpdate
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, exclude []int64, updates *mtproto.Updates) {
		pushed = append(pushed, pushedUpdate{userID: userID, exclude: append([]int64(nil), exclude...), updates: updates})
	}})

	ups, err := svc.SetHistoryTTL(ctx, 1, 77, ch.Id, 86400)
	if err != nil {
		t.Fatal(err)
	}
	newUp := findUpdate(ups, mtproto.Predicate_updateNewChannelMessage)
	if newUp == nil || newUp.GetPts_INT32() == 0 || newUp.GetPtsCount() != 1 {
		t.Fatalf("new channel message update = %#v", newUp)
	}
	if got := newUp.GetMessage_MESSAGE().GetAction(); got.GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL || got.GetPeriod() != 86400 || got.GetAutoSettingFrom() != nil {
		t.Fatalf("action = %#v", got)
	}
	ttlUp := findUpdate(ups, mtproto.Predicate_updatePeerHistoryTTL)
	if ttlUp == nil || ttlUp.GetTtlPeriod().GetValue() != 86400 || ttlUp.GetPeer_PEER().GetChannelId() != ch.Id {
		t.Fatalf("ttl update = %#v", ttlUp)
	}
	assertTTLAction(t, svc, ctx, 1, in, 86400)
	assertTTLAction(t, svc, ctx, 2, in, 86400)
	if !sawTTLPush(pushed, 2, nil) || !sawTTLPush(pushed, 1, []int64{77}) {
		t.Fatalf("ttl fanout = %#v", pushed)
	}

	ups, err = svc.SetHistoryTTL(ctx, 1, 77, ch.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	newUp = findUpdate(ups, mtproto.Predicate_updateNewChannelMessage)
	if got := newUp.GetMessage_MESSAGE().GetAction(); got.GetPeriod() != 0 || got.GetPredicateName() != mtproto.Predicate_messageActionSetMessagesTTL {
		t.Fatalf("disable action = %#v", got)
	}
	ttlUp = findUpdate(ups, mtproto.Predicate_updatePeerHistoryTTL)
	if ttlUp == nil || ttlUp.GetTtlPeriod() != nil {
		t.Fatalf("disable ttl update = %#v", ttlUp)
	}
	assertTTLAction(t, svc, ctx, 2, in, 0)
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.GetFullChat().GetTtlPeriod() != nil {
		t.Fatalf("full ttl after disable %+v %v", full.GetFullChat().GetTtlPeriod(), err)
	}
}

type pushedUpdate struct {
	userID  int64
	exclude []int64
	updates *mtproto.Updates
}

func findUpdate(ups *mtproto.Updates, pred string) *mtproto.Update {
	if ups == nil {
		return nil
	}
	for _, u := range ups.GetUpdates() {
		if u.GetPredicateName() == pred {
			return u
		}
	}
	return nil
}

func assertTTLAction(t *testing.T, svc *Service, ctx context.Context, user int64, in *mtproto.InputChannel, period int32) {
	t.Helper()
	hist, err := svc.History(ctx, user, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hist.GetMessages() {
		act := m.GetAction()
		if act.GetPredicateName() == mtproto.Predicate_messageActionSetMessagesTTL && act.GetPeriod() == period {
			return
		}
	}
	t.Fatalf("user %d history missing ttl action %d: %#v", user, period, hist.GetMessages())
}

func sawTTLPush(pushed []pushedUpdate, user int64, exclude []int64) bool {
	for _, p := range pushed {
		if p.userID != user || findUpdate(p.updates, mtproto.Predicate_updatePeerHistoryTTL) == nil {
			continue
		}
		if len(p.exclude) != len(exclude) {
			continue
		}
		ok := true
		for i := range exclude {
			if p.exclude[i] != exclude[i] {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}
