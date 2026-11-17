package user

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrPresenceNotFound is a missing user_presences row. It is not used for a
// database or cache failure, so callers can tell "never stored" from "read failed".
var ErrPresenceNotFound = status.Error(codes.NotFound, "PRESENCE_NOT_FOUND")
