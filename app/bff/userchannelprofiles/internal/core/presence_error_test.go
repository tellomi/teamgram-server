package core

import (
	"errors"
	"testing"

	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMissingPresenceIsTyped(t *testing.T) {
	if !missingPresence(userpb.ErrPresenceNotFound) {
		t.Fatal("PRESENCE_NOT_FOUND is a missing row")
	}
	if missingPresence(status.Error(codes.Internal, "USER_ID_INVALID")) {
		t.Fatal("USER_ID_INVALID must not be treated as a missing presence row")
	}
	if missingPresence(errors.New("mysql timeout")) {
		t.Fatal("a read failure is not a missing row")
	}
}
