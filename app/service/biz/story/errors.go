package story

import (
	"google.golang.org/grpc/status"

	"github.com/teamgram/proto/mtproto"
)

var (
	errStoryIDInvalid = status.Error(mtproto.ErrBadRequest, "STORY_ID_INVALID")
	errAlbumInvalid   = status.Error(mtproto.ErrBadRequest, "ALBUM_INVALID")
	errStoriesTooMuch = status.Error(mtproto.ErrBadRequest, "STORIES_TOO_MUCH")
	errBoostsRequired = status.Error(mtproto.ErrBadRequest, "BOOSTS_REQUIRED")
)
