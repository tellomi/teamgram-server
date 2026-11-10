package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// paintStoryUsers replaces stories_max_id copied from the user cache with the
// stories this viewer can still open. Chat fan-out builds users with ToUser,
// which does not do that on its own.
func paintStoryUsers(viewer int64, list *mtproto.MutableUsers, users []*mtproto.User) {
	if viewer == 0 || list == nil || len(users) == 0 || !mtproto.StoryRingsEnabled() {
		return
	}
	column := make(map[int64]int32, len(users))
	list.Visit(func(it *mtproto.ImmutableUser) {
		if it != nil {
			column[it.Id()] = mtproto.StoryColumn(it)
		}
	})
	mtproto.PaintUserStoryRings(context.Background(), viewer, users, column)
}
