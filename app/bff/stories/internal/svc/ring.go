package svc

import (
	"context"
	"errors"

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
	// UserGetMutableUsers loads forward contact rows. CheckPrivacy reads
	// those rows, and getImmutableUserV2 does not when To is empty.
	UserGetMutableUsers(ctx context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error)
}

// errStoryContacts means the forward contact rows were not loaded, so a
// viewer push would be a non-min user with "my contacts" fields removed.
var errStoryContacts = errors.New("story max: forward contacts unavailable")

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
	// Reverse contacts are who added this user. Their push is a non-min user,
	// so a missing forward edge would hide a "my contacts" photo, phone, and
	// last seen. Skip those pushes instead of sending that user. The stored
	// max is already corrected for the next fetch.
	if err = attachForwardContacts(ctx, users, im); err != nil {
		logx.WithContext(ctx).Errorf("story max contacts %d: %v", userID, err)
		return nil
	}
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

// attachForwardContacts fills im.Contacts from the same loader a new-message
// update uses. getImmutableUserV2 with an empty To list returns reverse
// contacts only. A later loader that selects forward peers from To still
// returns none for that call. ToUser then treats "my contacts" as deny.
func attachForwardContacts(ctx context.Context, users storyMaxUsers, im *mtproto.ImmutableUser) error {
	if im == nil {
		return nil
	}
	viewers := make([]int64, 0, len(im.GetReverseContacts()))
	seen := map[int64]bool{im.Id(): true}
	for _, contact := range im.GetReverseContacts() {
		viewer := contact.GetUserId()
		if viewer == 0 || seen[viewer] {
			continue
		}
		seen[viewer] = true
		viewers = append(viewers, viewer)
	}
	if len(viewers) == 0 {
		return nil
	}
	batch, err := users.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
		Id: append([]int64{im.Id()}, viewers...),
		To: viewers,
	})
	if err != nil || batch == nil {
		if err == nil {
			err = errStoryContacts
		}
		return err
	}
	for _, other := range batch.GetDatas() {
		if other != nil && other.Id() == im.Id() {
			im.Contacts = other.GetContacts()
			return nil
		}
	}
	return errStoryContacts
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
