package presence

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc"
)

// TouchInterval is how often one process rewrites last_seen_at for an
// active user. account.updateStatus is not counted: that RPC stores the
// explicit online or offline window itself.
const TouchInterval int64 = 60

// LastSeenStore reads and stores user_presences. The bff user client implements it.
type LastSeenStore interface {
	UserGetLastSeen(ctx context.Context, in *userpb.TLUserGetLastSeen) (*userpb.LastSeenData, error)
	UserUpdateLastSeen(ctx context.Context, in *userpb.TLUserUpdateLastSeen) (*mtproto.Bool, error)
}

// ShouldTouch reports whether last_seen_at should be rewritten.
// prev <= 0 is the first write for this process. now must be positive.
func ShouldTouch(prev, now, interval int64) bool {
	if now <= 0 || interval <= 0 {
		return false
	}
	if prev <= 0 {
		return true
	}
	return now-prev >= interval
}

// SkipMethod is account.updateStatus. Writing last seen from that RPC as
// well would replace an explicit offline with online.
func SkipMethod(fullMethod string) bool {
	return strings.HasSuffix(fullMethod, "/account_updateStatus")
}

// Toucher throttles last-seen writes inside one process.
type Toucher struct {
	mu       sync.Mutex
	last     map[int64]int64
	interval int64
	now      func() int64
	write    func(ctx context.Context, userID, now int64)
}

func NewToucher(interval int64, write func(ctx context.Context, userID, now int64)) *Toucher {
	if interval <= 0 {
		interval = TouchInterval
	}
	return &Toucher{
		last:     map[int64]int64{},
		interval: interval,
		now:      func() int64 { return time.Now().Unix() },
		write:    write,
	}
}

// Touch writes once per interval and does not wait for the store.
func (t *Toucher) Touch(ctx context.Context, userID int64) {
	if t == nil || t.write == nil || userID == 0 {
		return
	}
	now := t.now()
	t.mu.Lock()
	prev := t.last[userID]
	if !ShouldTouch(prev, now, t.interval) {
		t.mu.Unlock()
		return
	}
	t.last[userID] = now
	t.mu.Unlock()
	go t.write(ctx, userID, now)
}

// Forget drops the throttle so the next ordinary RPC can write. An
// updateStatus offline must not be hidden by a touch from a few seconds ago.
func (t *Toucher) Forget(userID int64) {
	if t == nil || userID == 0 {
		return
	}
	t.mu.Lock()
	delete(t.last, userID)
	t.mu.Unlock()
}

// activityExpires is what an ordinary RPC stores in user_presences.expires.
// An open online window is renewed for another UserOnlinePeriod. Offline,
// expired, and missing rows stay at 0 so a later getDifference does not
// show the user online.
func activityExpires(lastSeenAt int64, expires int32, now int64) int32 {
	if mtproto.PresenceOnline(now, lastSeenAt, expires) {
		return mtproto.UserOnlinePeriod
	}
	return 0
}

// recordActivity refreshes last_seen_at. It renews expires only while the
// stored window is still open. A read failure other than a missing row
// writes nothing, so a database error cannot turn someone online or offline.
func recordActivity(ctx context.Context, store LastSeenStore, userID, now int64) error {
	if store == nil || userID == 0 || now <= 0 {
		return nil
	}
	seen, err := store.UserGetLastSeen(ctx, &userpb.TLUserGetLastSeen{Id: userID})
	expires := int32(0)
	switch {
	case err == nil && seen != nil:
		expires = activityExpires(seen.GetLastSeenAt(), seen.GetExpires(), now)
	case err == nil || errors.Is(err, userpb.ErrPresenceNotFound):
		expires = 0
	default:
		return err
	}
	_, err = store.UserUpdateLastSeen(ctx, &userpb.TLUserUpdateLastSeen{
		Id:         userID,
		LastSeenAt: now,
		Expires:    expires,
	})
	return err
}

// Unary refreshes last seen on ordinary authenticated RPCs. An open online
// window is extended. An offline user only moves last_seen_at and keeps
// expires at 0. This does not push updateUserStatus.
func Unary(store LastSeenStore) grpc.UnaryServerInterceptor {
	toucher := NewToucher(TouchInterval, func(ctx context.Context, userID, now int64) {
		if store == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = recordActivity(ctx, store, userID, now)
	})
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var userID int64
		if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
			userID = md.UserId
		}
		method := ""
		if info != nil {
			method = info.FullMethod
		}
		if SkipMethod(method) {
			toucher.Forget(userID)
		} else {
			toucher.Touch(ctx, userID)
		}
		return handler(ctx, req)
	}
}
