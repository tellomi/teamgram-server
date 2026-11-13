package presence

import (
	"context"
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

// LastSeenWriter stores user_presences. The bff user client implements it.
type LastSeenWriter interface {
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

// Unary refreshes last seen on ordinary authenticated RPCs. The stored
// window is UserOnlinePeriod, the same value account.updateStatus online uses.
// This does not push updateUserStatus; the explicit status RPC still does.
func Unary(writer LastSeenWriter) grpc.UnaryServerInterceptor {
	toucher := NewToucher(TouchInterval, func(ctx context.Context, userID, now int64) {
		if writer == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, _ = writer.UserUpdateLastSeen(ctx, &userpb.TLUserUpdateLastSeen{
			Id:         userID,
			LastSeenAt: now,
			Expires:    mtproto.UserOnlinePeriod,
		})
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
