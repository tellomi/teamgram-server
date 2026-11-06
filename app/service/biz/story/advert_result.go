package story

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
)

// ErrAdvertCache means the user-service cache delete did not succeed.
// BoolFalse with a nil error is that failure, not a stored advertisement.
var ErrAdvertCache = errors.New("story max cache was not deleted")

// RejectUnstoredAdvert turns a user.setStoriesMaxId result into an error
// unless the cache delete succeeded. applyStoryMax and the restart drain
// both use it, so a BoolFalse cannot be cleared as if the cache were dropped.
func RejectUnstoredAdvert(ok *mtproto.Bool, err error) error {
	if err != nil {
		return err
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return ErrAdvertCache
	}
	return nil
}
