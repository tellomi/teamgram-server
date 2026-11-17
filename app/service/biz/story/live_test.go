package story

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestUseKeepsLiveStarter(t *testing.T) {
	t.Cleanup(func() {
		cur := Shared()
		cur.SetLiveStarter(nil)
		cur.SetCallAlive(nil)
		Use(New(NewMemoryStore()))
	})
	first := New(NewMemoryStore())
	Use(first)
	first.SetLiveStarter(func(context.Context, int64, int64, bool, int32, []int64) (int64, int64, *mtproto.Updates, error) {
		return 9, 8, nil, nil
	})
	first.SetCallAlive(func(int64) bool { return true })
	next := New(NewMemoryStore())
	Use(next)
	starter := next.liveStarter()
	if starter == nil {
		t.Fatal("replacing the story service dropped stories.startLive")
	}
	id, access, _, err := starter(context.Background(), 1, 0, false, 1, nil)
	if err != nil || id != 9 || access != 8 {
		t.Fatalf("starter %d %d %v", id, access, err)
	}
	if next.lives.alive == nil || !next.lives.alive(1) {
		t.Fatal("call-alive callback dropped")
	}
}

func TestEmptyLiveStubIsNotARingUntilTheCallExists(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	enableRings(t)
	owner := Owner{Kind: KindUser, ID: 1}
	up, err := svc.Send(ctx, SendRequest{
		Actor: 1, Owner: owner, Rules: rules("all"), RandomID: 3, Live: true, Caption: "live",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	if id == 0 || !up.Updates[len(up.Updates)-1].GetStory().GetLive() {
		t.Fatalf("start %+v", up)
	}
	stories, err := svc.GetPeerStories(ctx, 9, owner, 0)
	if err != nil || len(stories.GetStories().GetStories()) != 0 {
		t.Fatalf("stub visible %+v %v", stories, err)
	}
	maxes := svc.PeerMaxIDs(ctx, 9, []Owner{owner})
	if maxes[0].GetMaxId() != nil || maxes[0].GetLive() {
		t.Fatalf("stub ring %+v", maxes[0])
	}
	column := svc.liveColumn(ctx, 1)
	rendered := paintRing(ctx, 9, ringOwner(1, column), column)
	assertRing(t, rendered, 0, false)

	svc.bindLive(50, 7, owner, id)
	maxes = svc.PeerMaxIDs(ctx, 9, []Owner{owner})
	if maxes[0].GetMaxId().GetValue() != id || !maxes[0].GetLive() {
		t.Fatalf("on air %+v", maxes[0])
	}
	// MySQL stores this id in users.stories_max_id. Paint must then say live.
	rendered = paintRing(ctx, 9, ringOwner(1, id), id)
	assertRing(t, rendered, id, true)
	assertAgrees(t, svc, ctx, 9, 1, rendered)
}

func TestEndLiveDeletesEmptyStubAndPushes(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	var maxes []int32
	var pushed []*mtproto.Updates
	svc.SetHooks(Hooks{
		SetUserMax: func(_ context.Context, id int64, max int32) error {
			if id == 1 {
				maxes = append(maxes, max)
			}
			return nil
		},
		Push: func(_ context.Context, _ int64, _ []int64, up *mtproto.Updates) {
			pushed = append(pushed, up)
		},
	})
	owner := Owner{Kind: KindUser, ID: 1}
	up, err := svc.Send(ctx, SendRequest{
		Actor: 1, Owner: owner, Rules: rules("all"), RandomID: 4, Live: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	svc.bindLive(8, 9, owner, id)
	before := len(pushed)
	if err = svc.EndLiveByCall(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if svc.liveBound(owner, id) {
		t.Fatal("call still bound")
	}
	stories, err := svc.GetPeerStories(ctx, 2, owner, 0)
	if err != nil || len(stories.GetStories().GetStories()) != 0 {
		t.Fatalf("stories after end %+v %v", stories, err)
	}
	peer := svc.PeerMaxIDs(ctx, 2, []Owner{owner})
	if peer[0].GetMaxId() != nil || peer[0].GetLive() {
		t.Fatalf("ring after end %+v", peer[0])
	}
	if len(maxes) == 0 || maxes[len(maxes)-1] != 0 {
		t.Fatalf("advertisement %v", maxes)
	}
	var deleted bool
	for _, up := range pushed[before:] {
		for _, u := range up.GetUpdates() {
			if u.GetStory().GetPredicateName() == mtproto.Predicate_storyItemDeleted && u.GetStory().GetId() == id {
				deleted = true
			}
		}
	}
	if !deleted {
		t.Fatal("viewers were not pushed storyItemDeleted")
	}
	if err = svc.EndLiveByCall(ctx, 8); err != nil {
		t.Fatal(err)
	}
}

func TestEndLiveKeepsRecordingAsOrdinaryStory(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	owner := Owner{Kind: KindUser, ID: 1}
	up, err := svc.Send(ctx, SendRequest{
		Actor: 1, Owner: owner, Media: videoMedia(), Rules: rules("all"), RandomID: 5, Live: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	svc.bindLive(3, 1, owner, id)
	if err = svc.EndLive(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	maxes := svc.PeerMaxIDs(ctx, 4, []Owner{owner})
	if maxes[0].GetMaxId().GetValue() != id || maxes[0].GetLive() {
		t.Fatalf("recording %+v", maxes[0])
	}
	got, err := svc.GetPeerStories(ctx, 4, owner, 0)
	if err != nil || len(got.GetStories().GetStories()) != 1 || got.GetStories().GetStories()[0].GetLive() {
		t.Fatalf("story %+v %v", got, err)
	}
}

func TestRepairExpiresOrphanedLiveStub(t *testing.T) {
	svc, ctx, now := newSvc(t)
	var pushed []*mtproto.Updates
	svc.SetHooks(Hooks{Push: func(_ context.Context, _ int64, _ []int64, up *mtproto.Updates) {
		pushed = append(pushed, up)
	}})
	owner := Owner{Kind: KindUser, ID: 1}
	up, err := svc.Send(ctx, SendRequest{
		Actor: 1, Owner: owner, Rules: rules("all"), RandomID: 6, Live: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	*now += liveUnboundGrace - 1
	svc.ReleaseExpired(ctx)
	rec, _ := svc.store.Get(ctx, owner, id)
	if rec == nil || rec.Deleted {
		t.Fatal("stub repaired before the grace period")
	}
	before := len(pushed)
	*now += 2
	// The repair cursor walks users, then channels, then users again.
	svc.ReleaseExpired(ctx)
	svc.ReleaseExpired(ctx)
	rec, _ = svc.store.Get(ctx, owner, id)
	if rec == nil || !rec.Deleted || rec.Item.GetLive() {
		t.Fatalf("orphan stub %+v", rec)
	}
	maxes := svc.PeerMaxIDs(ctx, 2, []Owner{owner})
	if maxes[0].GetMaxId() != nil || maxes[0].GetLive() {
		t.Fatalf("ring after repair %+v", maxes[0])
	}
	var deleted bool
	for _, up := range pushed[before:] {
		for _, u := range up.GetUpdates() {
			if u.GetStory().GetPredicateName() == mtproto.Predicate_storyItemDeleted && u.GetStory().GetId() == id {
				deleted = true
			}
		}
	}
	if !deleted {
		t.Fatal("repair did not push storyItemDeleted")
	}
}

func TestBeginLiveBindsCallAndPushesLiveRing(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	var calls int
	svc.SetLiveStarter(func(_ context.Context, host, auth int64, rtmp bool, random int32, audience []int64) (int64, int64, *mtproto.Updates, error) {
		calls++
		if host != 1 || auth != 4 || !rtmp || random != 11 || len(audience) == 0 {
			t.Fatalf("starter host=%d auth=%d rtmp=%v random=%d audience=%v", host, auth, rtmp, random, audience)
		}
		return 70, 71, mtproto.MakeTLUpdates(&mtproto.Updates{
			Updates: []*mtproto.Update{
				mtproto.MakeTLUpdateGroupCall(&mtproto.Update{LiveStory: true, Call_GROUPCALL: mtproto.MakeTLGroupCall(&mtproto.GroupCall{Id: 70, AccessHash: 71}).To_GroupCall()}).To_Update(),
			},
		}).To_Updates(), nil
	})
	up, err := svc.BeginLive(ctx, SendRequest{
		Actor: 1, AuthKey: 4, Owner: Owner{Kind: KindUser, ID: 1},
		Rules: rules("all"), RandomID: 11,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("starter calls %d", calls)
	}
	var sawCall bool
	for _, u := range up.Updates {
		if u.GetLiveStory() && u.GetCall_GROUPCALL().GetId() == 70 {
			sawCall = true
		}
	}
	if !sawCall {
		t.Fatalf("response missing live call %+v", up)
	}
	maxes := svc.PeerMaxIDs(ctx, 1, []Owner{{Kind: KindUser, ID: 1}})
	if maxes[0].GetMaxId().GetValue() == 0 || !maxes[0].GetLive() {
		t.Fatalf("ring %+v", maxes[0])
	}
}
