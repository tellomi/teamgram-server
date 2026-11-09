package mtproto

import (
	"context"
	"sync/atomic"
)

// StoryRing is the active story a viewer may see on one user.
type StoryRing struct {
	MaxID int32
	Live  bool
}

// storyRingLookup loads live rings for one viewer. The story service installs
// it in the process that owns the story store. Other processes leave it unset
// and keep the users.stories_max_id column.
type storyRingLookupFunc func(ctx context.Context, viewer int64, ids []int64) (map[int64]StoryRing, error)

var (
	storyRingsOn atomic.Bool
	storyRingFn  atomic.Pointer[storyRingLookupFunc]
)

// EnableStoryRings turns on live per-viewer rings. A nil lookup turns it off.
func EnableStoryRings(lookup storyRingLookupFunc) {
	if lookup == nil {
		storyRingsOn.Store(false)
		storyRingFn.Store(nil)
		return
	}
	storyRingFn.Store(&lookup)
	storyRingsOn.Store(true)
}

// StoryRingsEnabled reports whether user objects are painted from live stories.
func StoryRingsEnabled() bool {
	return loadStoryRingLookup() != nil
}

func loadStoryRingLookup() storyRingLookupFunc {
	if !storyRingsOn.Load() {
		return nil
	}
	fn := storyRingFn.Load()
	if fn == nil {
		return nil
	}
	return *fn
}

// StoryColumn is users.stories_max_id. Zero means the user object can skip
// the live story query.
func StoryColumn(im *ImmutableUser) int32 {
	if im == nil || im.User == nil {
		return 0
	}
	return im.User.StoriesMaxId
}

// ApplyStoryRing writes recentStory for one non-min user. max <= 0 omits it
// and sets stories_unavailable.
func ApplyStoryRing(user *User, max int32, live bool) {
	if user == nil || user.Deleted || user.Min || user.Bot {
		return
	}
	if max > 0 {
		user.StoriesUnavailable = false
		user.StoriesMaxId_FLAGINT32 = nil
		user.StoriesMaxId_FLAGRECENTSTORY = MakeTLRecentStory(&RecentStory{
			Live:  live,
			MaxId: MakeFlagsInt32(max),
		}).To_RecentStory()
		return
	}
	user.StoriesUnavailable = true
	user.StoriesMaxId_FLAGINT32 = nil
	user.StoriesMaxId_FLAGRECENTSTORY = nil
}

// PaintUserStoryRings sets recentStory from the live per-viewer max.
// column is users.stories_max_id keyed by user id. A zero value skips the
// query and clears the ring. A nil map queries every non-min user.
// A lookup error leaves the queried users as they were rendered.
func PaintUserStoryRings(ctx context.Context, viewer int64, users []*User, column map[int64]int32) {
	lookup := loadStoryRingLookup()
	if lookup == nil || viewer == 0 || len(users) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := make([]int64, 0, len(users))
	for _, user := range users {
		if !storyPaintable(user) {
			continue
		}
		if column != nil && column[user.Id] <= 0 {
			ApplyStoryRing(user, 0, false)
			continue
		}
		query = append(query, user.Id)
	}
	if len(query) == 0 {
		return
	}
	rings, err := lookup(ctx, viewer, query)
	if err != nil {
		return
	}
	queried := make(map[int64]bool, len(query))
	for _, id := range query {
		queried[id] = true
	}
	for _, user := range users {
		if user == nil || !queried[user.Id] {
			continue
		}
		ring := rings[user.Id]
		ApplyStoryRing(user, ring.MaxID, ring.Live)
	}
}

func storyPaintable(user *User) bool {
	return user != nil && user.Id != 0 && !user.Deleted && !user.Min && !user.Bot
}
