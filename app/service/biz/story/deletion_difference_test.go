package story

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMissedDeletionReachesDialogPeerAndChangesTrayHash(t *testing.T) {
	svc, ctx, now := newSvc(t)
	const owner, dialogPeer, contact, allow, stranger int64 = 1, 5, 2, 6, 9
	svc.MarkContact(owner, contact)
	svc.SetHooks(Hooks{
		DialogPeers: func(_ context.Context, id int64, _ int) ([]int64, error) {
			if id == owner {
				return []int64{dialogPeer}, nil
			}
			return nil, nil
		},
	})
	offline := *now - 30

	empty, err := svc.GetAll(ctx, dialogPeer, false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if empty.State != "s:af63bd4c8601b7df" {
		t.Fatalf("empty tray state %q", empty.State)
	}

	up, err := svc.Send(ctx, SendRequest{
		Actor: owner, Owner: Owner{Kind: KindUser, ID: owner},
		Rules: rules("all"), RandomID: 3, Live: true, Caption: "live",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	if err = svc.EndLive(ctx, Owner{Kind: KindUser, ID: owner}, id); err != nil {
		t.Fatal(err)
	}

	assertDeleted := func(viewer int64, want bool) {
		t.Helper()
		ups := svc.DifferenceDeletions(ctx, viewer, offline)
		got := storyDeleted(ups, owner, id)
		if got != want {
			t.Fatalf("viewer %d deletion=%v ups=%d", viewer, got, len(ups))
		}
	}
	assertDeleted(dialogPeer, true)
	assertDeleted(contact, true)
	assertDeleted(owner, true)
	assertDeleted(stranger, false)
	// The date this difference stores is after the deletion, so the next
	// catch-up does not repeat it.
	if again := svc.DifferenceDeletions(ctx, dialogPeer, *now); len(again) != 0 {
		t.Fatalf("repeated %d deletions", len(again))
	}

	changed, err := svc.GetAll(ctx, dialogPeer, false, false, empty.State)
	if err != nil {
		t.Fatal(err)
	}
	if changed.GetPredicateName() == mtproto.Predicate_stories_allStoriesNotModified || changed.State == empty.State {
		t.Fatalf("tray stayed NotModified at %q", changed.GetState())
	}
	same, err := svc.GetAll(ctx, dialogPeer, false, false, changed.State)
	if err != nil || same.GetPredicateName() != mtproto.Predicate_stories_allStoriesNotModified {
		t.Fatalf("stable reread %+v %v", same, err)
	}
	strangerTray, err := svc.GetAll(ctx, stranger, false, false, empty.State)
	if err != nil || strangerTray.GetPredicateName() != mtproto.Predicate_stories_allStoriesNotModified {
		t.Fatalf("stranger tray %+v %v", strangerTray, err)
	}

	// A second deletion in the same second still moves the watermark.
	second := send(t, svc, ctx, owner, photoMedia(), rules("all"), 4, "keep", false)
	secondID := storyID(t, second)
	if _, err = svc.Delete(ctx, owner, 0, Owner{Kind: KindUser, ID: owner}, 0, []int32{secondID}); err != nil {
		t.Fatal(err)
	}
	afterSecond, err := svc.GetAll(ctx, dialogPeer, false, false, changed.State)
	if err != nil || afterSecond.GetPredicateName() == mtproto.Predicate_stories_allStoriesNotModified {
		t.Fatalf("second deletion did not move the tray state %+v %v", afterSecond, err)
	}

	// Contacts-only and allow-list stories stay inside that audience.
	// A saved recording is not a deletion.
	*now++
	recording, err := svc.Send(ctx, SendRequest{
		Actor: owner, Owner: Owner{Kind: KindUser, ID: owner},
		Media: videoMedia(), Rules: rules("all"), RandomID: 8, Live: true, Caption: "rec",
	})
	if err != nil {
		t.Fatal(err)
	}
	recID := storyID(t, recording)
	if err = svc.EndLive(ctx, Owner{Kind: KindUser, ID: owner}, recID); err != nil {
		t.Fatal(err)
	}
	if storyDeleted(svc.DifferenceDeletions(ctx, dialogPeer, *now-1), owner, recID) {
		t.Fatal("ending a recorded live was stored as a deletion")
	}

	allowed := send(t, svc, ctx, owner, photoMedia(), allowUsers(allow), 9, "picked", false)
	allowID := storyID(t, allowed)
	if _, err = svc.Delete(ctx, owner, 0, Owner{Kind: KindUser, ID: owner}, 0, []int32{allowID}); err != nil {
		t.Fatal(err)
	}
	if !storyDeleted(svc.DifferenceDeletions(ctx, allow, *now-1), owner, allowID) {
		t.Fatal("allow-list viewer missed the deletion")
	}
	if storyDeleted(svc.DifferenceDeletions(ctx, dialogPeer, *now-1), owner, allowID) {
		t.Fatal("dialog peer received a private deletion")
	}
}

func storyDeleted(ups []*mtproto.Update, owner int64, id int32) bool {
	for _, u := range ups {
		item := u.GetStory()
		if u.GetPredicateName() == mtproto.Predicate_updateStory &&
			u.GetPeer_PEER().GetUserId() == int64(owner) &&
			item.GetPredicateName() == mtproto.Predicate_storyItemDeleted &&
			item.GetId() == id {
			return true
		}
	}
	return false
}
