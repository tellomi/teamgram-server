package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

// TestOfflineViewerGetDifferenceCarriesStoryItemDeleted is the reconnect
// path of updates.getDifference. The viewer is not a contact. They only
// share a private dialog, so stories.getAllStories does not list the owner.
// The live push is missed. The difference merge used by UpdatesGetDifference
// has to carry storyItemDeleted and keep the pts the client already had.
func TestOfflineViewerGetDifferenceCarriesStoryItemDeleted(t *testing.T) {
	svc := story.New(story.NewMemoryStore())
	prev := story.Shared()
	story.Use(svc)
	t.Cleanup(func() { story.Use(prev) })

	ctx := context.Background()
	const owner, viewer, stranger int64 = 1, 5, 9
	svc.SetHooks(story.Hooks{
		DialogPeers: func(_ context.Context, id int64, _ int) ([]int64, error) {
			if id == owner {
				return []int64{viewer}, nil
			}
			return nil, nil
		},
	})
	up, err := svc.Send(ctx, story.SendRequest{
		Actor: owner, Owner: story.Owner{Kind: story.KindUser, ID: owner},
		Rules:    []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()},
		RandomID: 3, Live: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var storyID int32
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID {
			storyID = u.GetId_INT32()
		}
	}
	if storyID == 0 {
		t.Fatal("missing story id")
	}
	offlineSince := int32(time.Now().Unix()) - 30
	if err = svc.EndLive(ctx, story.Owner{Kind: story.KindUser, ID: owner}, storyID); err != nil {
		t.Fatal(err)
	}

	empty := mtproto.MakeTLUpdatesDifferenceEmpty(&mtproto.Updates_Difference{
		Date: offlineSince,
		Seq:  4,
	}).To_Updates_Difference()
	caughtUp := append(
		chatbox.Shared().DifferenceReactions(ctx, viewer, offlineSince),
		story.Shared().DifferenceDeletions(ctx, viewer, offlineSince)...,
	)
	got := withReactionUpdates(empty, caughtUp, 120, 7)
	if got.GetPredicateName() != mtproto.Predicate_updates_difference {
		t.Fatalf("difference %s", got.GetPredicateName())
	}
	state := got.GetState()
	if state.GetPts() != 120 || state.GetQts() != 7 || state.GetSeq() != 4 || state.GetDate() <= offlineSince {
		t.Fatalf("state %+v", state)
	}
	if !storyDeletedUpdate(got.GetOtherUpdates(), owner, storyID) {
		t.Fatalf("other_updates %+v", got.GetOtherUpdates())
	}
	if again := story.Shared().DifferenceDeletions(ctx, viewer, state.GetDate()); len(again) != 0 {
		t.Fatalf("deletion repeated after the new date: %d", len(again))
	}
	if ups := story.Shared().DifferenceDeletions(ctx, stranger, offlineSince); len(ups) != 0 {
		t.Fatalf("stranger received %d deletions", len(ups))
	}
}

func storyDeletedUpdate(ups []*mtproto.Update, owner int64, id int32) bool {
	for _, u := range ups {
		item := u.GetStory()
		if u.GetPredicateName() == mtproto.Predicate_updateStory &&
			u.GetPeer_PEER().GetUserId() == owner &&
			item.GetPredicateName() == mtproto.Predicate_storyItemDeleted &&
			item.GetId() == id {
			return true
		}
	}
	return false
}
