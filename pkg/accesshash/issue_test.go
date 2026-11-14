package accesshash

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPaintUsersOnlyWhenIssuing(t *testing.T) {
	defer SetMode(ModeOff)()
	defer SetUserLookup(nil)
	SetUserLookup(func(ctx context.Context, ids []int64) (map[int64]int64, error) {
		return map[int64]int64{4: 44, 5: 0}, nil
	})
	users := []*mtproto.User{
		mtproto.MakeTLUser(&mtproto.User{Id: 4, Min: true}).To_User(),
		mtproto.MakeTLUser(&mtproto.User{Id: 5, Min: true}).To_User(),
	}
	PaintUsers(context.Background(), users)
	if users[0].GetAccessHash().GetValue() != 0 || !users[0].GetMin() {
		t.Fatal("off changed the user")
	}

	defer SetMode(ModeLog)()
	PaintUsers(context.Background(), users)
	if users[0].GetAccessHash().GetValue() != 44 || users[0].GetMin() {
		t.Fatalf("user 4: hash %d min %v", users[0].GetAccessHash().GetValue(), users[0].GetMin())
	}
	if users[1].GetAccessHash().GetValue() != 0 {
		t.Fatal("stored 0 was written")
	}
}

func TestDropZero(t *testing.T) {
	got := DropZero([]int64{0, 4, 0, 5})
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("%v", got)
	}
}

func TestEnforceRejectsStoredZero(t *testing.T) {
	defer SetMode(ModeEnforce)()
	if AllowProvided(0, 0) || AllowProvided(9, 0) || AllowChannel(0, 0) || AllowChannel(9, 0) {
		t.Fatal("stored 0 must not match")
	}
	if !AllowProvided(9, 9) || !AllowChannel(9, 9) {
		t.Fatal("real hash")
	}
}
