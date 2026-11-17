package stories_helper

import (
	"testing"

	files_helper "github.com/teamgram/teamgram-server/app/bff/files"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

// TestCombinedBFFKeepsStoryHooks follows app/bff/bff: stories_helper.New, then
// files_helper.New. The files helper may fill hooks that are still nil. It
// must not replace the stories fanout.
func TestCombinedBFFKeepsStoryHooks(t *testing.T) {
	story.Use(story.New(story.NewMemoryStore()))
	_ = New(Config{})
	before := story.Shared().HooksSet()
	if !before.Push || !before.ContactIDs || !before.SetUserMax || !before.SetStoriesHidden || !before.Username || !before.MatchName || !before.IsContact || !before.IsCloseFriend || !before.IsBlocked {
		t.Fatalf("stories helper: %+v", before)
	}
	_ = files_helper.New(files_helper.Config{}, nil, nil)
	got := story.Shared().HooksSet()
	if !got.Push || !got.ContactIDs || !got.SetUserMax || !got.SetStoriesHidden || !got.Username || !got.MatchName {
		t.Fatalf("files helper replaced story hooks: %+v", got)
	}
	if !got.IsContact || !got.IsCloseFriend || !got.IsBlocked {
		t.Fatalf("privacy hooks missing: %+v", got)
	}
	if got != before {
		t.Fatalf("hook set changed after files helper\nbefore %+v\nafter  %+v", before, got)
	}
}
