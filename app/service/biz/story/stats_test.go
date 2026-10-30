package story

import (
	"testing"

	"google.golang.org/grpc/status"
)

func TestStoryStats(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	up := send(t, svc, ctx, 3, photoMedia(), rules("all"), 1, "hi", false)
	id := storyID(t, up)
	owner := Owner{Kind: KindUser, ID: 3}
	if _, err := svc.StoryStats(ctx, 4, owner, 0, id); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatal(err)
	}
	stats, err := svc.StoryStats(ctx, 3, owner, 0, id)
	if err != nil || stats.GetViewsGraph().GetToken() == "" {
		t.Fatalf("%#v %v", stats, err)
	}
	fwd, err := svc.StoryPublicForwards(ctx, 3, owner, 0, id, "", 10)
	if err != nil || fwd.GetCount() != 0 {
		t.Fatalf("forwards %#v %v", fwd, err)
	}
	if _, err := svc.StoryStats(ctx, 3, owner, 0, 0); status.Convert(err).Message() != "STORY_ID_INVALID" {
		t.Fatal(err)
	}
	empty := Owner{Kind: KindUser, ID: 8}
	if _, err := svc.StoryStats(ctx, 8, empty, 0, 1); status.Convert(err).Message() != "STORIES_NEVER_CREATED" {
		t.Fatal(err)
	}
}
