package story

import (
	"context"
	"testing"
)

func TestViewsListMarksStoryBlock(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	blocked := map[int64]bool{2: true}
	svc.SetHooks(Hooks{
		StoryBlockedFrom: func(_ context.Context, owner, viewer int64) bool {
			return owner == 1 && blocked[viewer]
		},
	})
	up := send(t, svc, ctx, 1, photoMedia(), rules("all"), 7, "view me", false)
	id := storyID(t, up)
	owner := Owner{Kind: KindUser, ID: 1}
	if err := svc.IncrementViews(ctx, 2, owner, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	if err := svc.IncrementViews(ctx, 3, owner, 0, []int32{id}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ViewsList(ctx, ViewsQuery{Actor: 1, Owner: owner, ID: id, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	flags := map[int64]bool{}
	for _, view := range list.GetViews() {
		flags[view.GetUserId()] = view.GetBlockedMyStoriesFrom()
	}
	if !flags[2] {
		t.Fatalf("blocked viewer flag = %v", flags)
	}
	if flags[3] {
		t.Fatalf("open viewer was flagged: %v", flags)
	}
	delete(blocked, 2)
	again, err := svc.ViewsList(ctx, ViewsQuery{Actor: 1, Owner: owner, ID: id, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range again.GetViews() {
		if view.GetUserId() == 2 && view.GetBlockedMyStoriesFrom() {
			t.Fatal("flag stayed after the story block was removed")
		}
	}
}
