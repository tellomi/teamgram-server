package core

import (
	"errors"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestPresenceReadErrorIsTyped(t *testing.T) {
	if !errors.Is(presenceReadError(sqlc.ErrNotFound), user.ErrPresenceNotFound) {
		t.Fatal("missing row must be PRESENCE_NOT_FOUND")
	}
	if !errors.Is(presenceReadError(nil), user.ErrPresenceNotFound) {
		t.Fatal("nil row without an error is still not found")
	}
	readErr := errors.New("mysql timeout")
	if errors.Is(presenceReadError(readErr), user.ErrPresenceNotFound) {
		t.Fatal("a read failure must not look like a missing row")
	}
	if !errors.Is(presenceReadError(readErr), readErr) {
		t.Fatal("read failure must be returned as itself")
	}
}
