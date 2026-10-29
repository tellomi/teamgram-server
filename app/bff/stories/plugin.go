package stories_helper

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

// Plugin fills userFull story fields from the shared story service.
type Plugin struct{}

func (Plugin) GetStoriesPinnedAvailable(ctx context.Context, peerUserID, toSelfUserID int64) bool {
	return story.Shared().PinnedAvailable(ctx, peerUserID, toSelfUserID)
}

func (Plugin) GetBlockedMyStoriesFrom(ctx context.Context, peerUserID, toSelfUserID int64) bool {
	// Layer 232 has no method that blocks one user from seeing your stories
	// apart from the per-story privacy rules, which already hide the items.
	return false
}

func (Plugin) GetActiveStories(ctx context.Context, peerUserID, toSelfUserID int64) *mtproto.PeerStories {
	return story.Shared().ActivePeerStories(ctx, peerUserID, toSelfUserID)
}
