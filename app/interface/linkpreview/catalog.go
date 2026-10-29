package linkpreview

import (
	"context"
	"errors"

	"github.com/teamgram/proto/mtproto"
)

// storedPeer is a public username target. ID is internal and is not serialized.
type storedPeer struct {
	ID          int64
	Kind        string
	Title       string
	Username    string
	About       string
	AvatarPath  string
	PhotoID     int64
	Verified    bool
	Scam        bool
	Fake        bool
	Premium     bool
	MemberCount *int32
	Broadcast   bool
	Megagroup   bool
}

type storedInvite struct {
	Title         string
	About         string
	AvatarPath    string
	PhotoID       int64
	MemberCount   int32
	RequestNeeded bool
	Broadcast     bool
	Megagroup     bool
}

type storedMsg struct {
	Msg *mtproto.Message
}

type storedStory struct {
	Item    *mtproto.StoryItem
	Deleted bool
}

type storedPack struct {
	ShortName  string
	Title      string
	AvatarPath string
	Count      int32
	Emoji      bool
}

// Catalog reads public records. Implementations return errNotFound when the
// record is missing or is not public.
type Catalog interface {
	PeerByUsername(ctx context.Context, username string) (*storedPeer, error)
	PeerByChannelID(ctx context.Context, id int64) (*storedPeer, error)
	Invite(ctx context.Context, hash string) (*storedInvite, error)
	Message(ctx context.Context, channelID int64, id int32) (*storedMsg, error)
	Latest(ctx context.Context, channelID int64, limit int) ([]storedMsg, error)
	Story(ctx context.Context, ownerKind int8, ownerID int64, id int32) (*storedStory, error)
	Pack(ctx context.Context, short string, emoji bool) (*storedPack, error)
	FilePath(ctx context.Context, photoID, documentID int64) (string, error)
}

type statusError struct {
	Code  string
	Path  string
	cause error
}

func (e *statusError) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code
}

func (e *statusError) Unwrap() error { return e.cause }

var (
	errNotFound = &statusError{Code: ErrNotFound}
	errInvalid  = &statusError{Code: ErrInvalidPath}
	errInternal = &statusError{Code: ErrInternal}
)

func errorCode(err error) string {
	var se *statusError
	if errors.As(err, &se) && se.Code != "" {
		return se.Code
	}
	if err == nil {
		return ""
	}
	return ErrInternal
}

func withCause(code string, cause error) error {
	if cause == nil {
		return &statusError{Code: code}
	}
	return &statusError{Code: code, cause: cause}
}
