package presence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestShouldTouchThrottles(t *testing.T) {
	if !ShouldTouch(0, 100, TouchInterval) {
		t.Fatal("first write")
	}
	if ShouldTouch(100, 100+TouchInterval-1, TouchInterval) {
		t.Fatal("inside the window")
	}
	if !ShouldTouch(100, 100+TouchInterval, TouchInterval) {
		t.Fatal("at the window")
	}
	if ShouldTouch(100, 0, TouchInterval) || ShouldTouch(100, 200, 0) {
		t.Fatal("missing clock or interval")
	}
}

func TestSkipUpdateStatus(t *testing.T) {
	if !SkipMethod("/mtproto.RPCUserChannelProfiles/account_updateStatus") {
		t.Fatal("updateStatus sets the row itself")
	}
	if SkipMethod("/mtproto.RPCMessages/messages_sendMessage") {
		t.Fatal("an ordinary RPC should refresh last seen")
	}
}

func TestToucherWritesOnceUntilForgotten(t *testing.T) {
	var got []int64
	var mu sync.Mutex
	tchr := NewToucher(TouchInterval, func(_ context.Context, userID, now int64) {
		mu.Lock()
		got = append(got, userID, now)
		mu.Unlock()
	})
	tchr.now = func() int64 { return 1_000 }
	tchr.Touch(context.Background(), 7)
	tchr.Touch(context.Background(), 0)
	tchr.Touch(context.Background(), 7)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if len(got) != 2 || got[0] != 7 || got[1] != 1_000 {
		mu.Unlock()
		t.Fatalf("writes %v", got)
	}
	mu.Unlock()

	tchr.Forget(7)
	tchr.Touch(context.Background(), 7)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 4 || got[2] != 7 || got[3] != 1_000 {
		t.Fatalf("forget should allow another write, got %v", got)
	}
}

func TestActivityExpiresRenewsOnlyAnOpenWindow(t *testing.T) {
	const seen int64 = 1_000
	if got := activityExpires(seen, mtproto.UserOnlinePeriod, seen+10); got != mtproto.UserOnlinePeriod {
		t.Fatalf("open window expires %d", got)
	}
	if got := activityExpires(seen, mtproto.UserOnlinePeriod, seen+int64(mtproto.UserOnlinePeriod)); got != 0 {
		t.Fatalf("expired window expires %d", got)
	}
	if got := activityExpires(seen, 0, seen+10); got != 0 {
		t.Fatalf("explicit offline expires %d", got)
	}
	if got := activityExpires(0, 0, seen); got != 0 {
		t.Fatalf("missing row expires %d", got)
	}
}

// getDifference after account.updateStatus(offline) must not paint the user
// online for another UserOnlinePeriod.
func TestOrdinaryRPCAfterOfflineStaysOffline(t *testing.T) {
	const (
		userID = int64(7)
		now    = int64(1_700_000_100)
	)
	store := &memPresence{row: &userpb.LastSeenData{UserId: userID, LastSeenAt: now - 30, Expires: 0}}
	if err := recordActivity(context.Background(), store, userID, now); err != nil {
		t.Fatal(err)
	}
	if store.writes != 1 || store.row.GetLastSeenAt() != now || store.row.GetExpires() != 0 {
		t.Fatalf("stored seen %d expires %d writes %d", store.row.GetLastSeenAt(), store.row.GetExpires(), store.writes)
	}
	if mtproto.PresenceOnline(now, store.row.GetLastSeenAt(), store.row.GetExpires()) {
		t.Fatal("offline user became online")
	}
	shown := mtproto.PresenceStatus(mtproto.PresenceCursor(store.row.GetLastSeenAt(), store.row.GetExpires()), mtproto.UserOnlinePeriod, true, now)
	if shown.GetPredicateName() != mtproto.Predicate_userStatusOffline {
		t.Fatalf("viewer status %s", shown.GetPredicateName())
	}
}

func TestOrdinaryRPCRenewsAnOpenOnlineWindow(t *testing.T) {
	const (
		userID = int64(7)
		seen   = int64(1_700_000_000)
		now    = seen + 20
	)
	store := &memPresence{row: &userpb.LastSeenData{UserId: userID, LastSeenAt: seen, Expires: mtproto.UserOnlinePeriod}}
	if err := recordActivity(context.Background(), store, userID, now); err != nil {
		t.Fatal(err)
	}
	if store.row.GetLastSeenAt() != now || store.row.GetExpires() != mtproto.UserOnlinePeriod {
		t.Fatalf("renewed seen %d expires %d", store.row.GetLastSeenAt(), store.row.GetExpires())
	}
	if !mtproto.PresenceOnline(now, store.row.GetLastSeenAt(), store.row.GetExpires()) {
		t.Fatal("open window was not renewed")
	}
}

func TestOrdinaryRPCDoesNotWriteWhenTheReadFails(t *testing.T) {
	store := &memPresence{err: errors.New("mysql timeout")}
	if err := recordActivity(context.Background(), store, 7, 1_000); err == nil {
		t.Fatal("read failure was ignored")
	}
	if store.writes != 0 {
		t.Fatalf("writes %d", store.writes)
	}
}

func TestOrdinaryRPCOfAMissingRowStaysOffline(t *testing.T) {
	store := &memPresence{err: userpb.ErrPresenceNotFound}
	if err := recordActivity(context.Background(), store, 7, 1_000); err != nil {
		t.Fatal(err)
	}
	if store.writes != 1 || store.row.GetExpires() != 0 || store.row.GetLastSeenAt() != 1_000 {
		t.Fatalf("stored %+v writes %d", store.row, store.writes)
	}
}

type memPresence struct {
	row    *userpb.LastSeenData
	err    error
	writes int
}

func (m *memPresence) UserGetLastSeen(context.Context, *userpb.TLUserGetLastSeen) (*userpb.LastSeenData, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.row, nil
}

func (m *memPresence) UserUpdateLastSeen(_ context.Context, in *userpb.TLUserUpdateLastSeen) (*mtproto.Bool, error) {
	m.writes++
	m.row = &userpb.LastSeenData{UserId: in.GetId(), LastSeenAt: in.GetLastSeenAt(), Expires: in.GetExpires()}
	m.err = nil
	return mtproto.BoolTrue, nil
}
