package webpage

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// UserCard is the public profile used for a telegram_user or telegram_bot page.
// Photo is set only when the viewer is allowed to see the profile photo.
// It is the stored Photo, including the access_hash upload.getFile checks.
type UserCard struct {
	ID       int64
	Title    string
	About    string
	Username string
	Bot      bool
	Photo    *mtproto.Photo
	User     *mtproto.User
}

// GroupCard is a basic-group invite (tell.cc/+hash, tell.cc/joinchat/hash).
type GroupCard struct {
	Title   string
	About   string
	Members int32
	Photo   *mtproto.Photo
	Chat    *mtproto.Chat
}

// UserResolver looks up a public username. ok is false when the name is
// not a user. A lookup error is ok false, not a failed RPC for the caller.
type UserResolver func(ctx context.Context, username string) (UserCard, bool)

// GroupResolver looks up a basic-group invite hash. ok is false when the
// hash is not a basic group.
type GroupResolver func(ctx context.Context, hash string) (GroupCard, bool)

// PhotoLoader loads a stored photo by id. Nil means the id is unknown.
// The returned Photo keeps the access_hash issued with the file.
type PhotoLoader func(ctx context.Context, photoID int64) *mtproto.Photo

var (
	userMu      sync.RWMutex
	lookupUser  UserResolver
	groupMu     sync.RWMutex
	lookupGroup GroupResolver
	photoMu     sync.RWMutex
	loadPhoto   PhotoLoader
)

// BindUserResolver installs the user directory used for telegram_user cards.
// The messages service calls this at startup. Tests replace it.
func BindUserResolver(fn UserResolver) {
	userMu.Lock()
	lookupUser = fn
	userMu.Unlock()
}

func lookupUserCard(ctx context.Context, username string) (UserCard, bool) {
	userMu.RLock()
	fn := lookupUser
	userMu.RUnlock()
	if fn == nil {
		return UserCard{}, false
	}
	return fn(ctx, username)
}

// BindGroupResolver installs the basic-group invite lookup.
func BindGroupResolver(fn GroupResolver) {
	groupMu.Lock()
	lookupGroup = fn
	groupMu.Unlock()
}

func lookupGroupCard(ctx context.Context, hash string) (GroupCard, bool) {
	groupMu.RLock()
	fn := lookupGroup
	groupMu.RUnlock()
	if fn == nil {
		return GroupCard{}, false
	}
	return fn(ctx, hash)
}

// BindPhotoLoader installs the media lookup for profile and chat photos.
func BindPhotoLoader(fn PhotoLoader) {
	photoMu.Lock()
	loadPhoto = fn
	photoMu.Unlock()
}

func storedPhoto(ctx context.Context, id int64) *mtproto.Photo {
	if id == 0 {
		return nil
	}
	photoMu.RLock()
	fn := loadPhoto
	photoMu.RUnlock()
	if fn == nil {
		return nil
	}
	photo := fn(ctx, id)
	if photo == nil || photo.GetId() == 0 || photo.GetPredicateName() == mtproto.Predicate_photoEmpty {
		return nil
	}
	return photo
}
