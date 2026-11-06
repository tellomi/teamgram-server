package svc

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/logx"
)

// storyMaxUsers is the user-service surface that stores and re-reads the
// active story id. The bff user client implements it.
type storyMaxUsers interface {
	UserSetStoriesMaxId(ctx context.Context, in *user.TLUserSetStoriesMaxId) (*mtproto.Bool, error)
	UserGetImmutableUserV2(ctx context.Context, in *user.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error)
}

// storyPusher delivers an updates container to one user.
type storyPusher interface {
	SyncPushUpdates(ctx context.Context, in *sync.TLSyncPushUpdates) (*mtproto.Void, error)
}

// applyStoryMax stores users.stories_max_id and sends the corrected user to
// the sessions that draw a chat-list ring from it. recentStory has no
// expire_date, so an online client keeps the previous value until a new user
// object arrives.
func applyStoryMax(ctx context.Context, users storyMaxUsers, syncer storyPusher, userID int64, maxID int32) error {
	if users == nil || userID == 0 {
		return nil
	}
	if _, err := users.UserSetStoriesMaxId(ctx, &user.TLUserSetStoriesMaxId{UserId: userID, Id: maxID}); err != nil {
		logx.WithContext(ctx).Errorf("story max id: %v", err)
		return err
	}
	if syncer == nil {
		return nil
	}
	im, err := users.UserGetImmutableUserV2(ctx, &user.TLUserGetImmutableUserV2{
		Id:      userID,
		Privacy: true,
		HasTo:   true,
	})
	if err != nil || im == nil || im.User == nil {
		// The column is already updated. A later user fetch omits the ring
		// even if this push cannot be built.
		logx.WithContext(ctx).Errorf("story max user %d: %v", userID, err)
		return nil
	}
	pushStoryUser(ctx, syncer, userID, userID, im.ToSelfUser())
	seen := map[int64]bool{userID: true}
	for _, contact := range im.GetReverseContacts() {
		viewer := contact.GetUserId()
		if viewer == 0 || seen[viewer] {
			continue
		}
		seen[viewer] = true
		pushStoryUser(ctx, syncer, viewer, userID, im.ToUser(viewer))
	}
	return nil
}

func pushStoryUser(ctx context.Context, syncer storyPusher, to, userID int64, rendered *mtproto.User) {
	if syncer == nil || to == 0 || rendered == nil {
		return
	}
	_, _ = syncer.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
		UserId: to,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			[]*mtproto.User{rendered},
			mtproto.MakeTLUpdateUser(&mtproto.Update{UserId: userID}).To_Update(),
		),
	})
}
