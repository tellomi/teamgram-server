// Package stagelog records per-stage timings for one MTProto request.
// It is off unless TELLOMI_STAGE_TIMING=1. Production can turn it on for a
// stress run and grep "stage ".
package stagelog

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/metadata"
)

// GatewayHeader is the gRPC metadata key carrying the gateway receive time
// as Unix nanoseconds. It is attached only while stage timing is on.
const GatewayHeader = "x-tellomi-gw-ns"

type marks struct {
	gateway  time.Time
	enqueued time.Time
	loop     time.Time
}

type marksKey struct{}

var (
	enabledOnce sync.Once
	enabled     bool
)

// On reports whether stage lines should be written.
// TELLOMI_STAGE_TIMING=1 enables them. Any other value, including unset, leaves them off.
func On() bool {
	enabledOnce.Do(func() {
		enabled = os.Getenv("TELLOMI_STAGE_TIMING") == "1"
	})
	return enabled
}

// Infof writes one "stage ..." line when timing is on.
func Infof(format string, args ...any) {
	if !On() {
		return
	}
	logx.Infof("stage "+format, args...)
}

// AppendGateway stamps the gateway clock onto the outgoing session RPC.
func AppendGateway(ctx context.Context) context.Context {
	if !On() || ctx == nil {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, GatewayHeader, strconv.FormatInt(time.Now().UnixNano(), 10))
}

// StampGateway copies the gateway clock from incoming gRPC metadata into ctx.
func StampGateway(ctx context.Context) context.Context {
	if !On() || ctx == nil {
		return ctx
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	vals := md.Get(GatewayHeader)
	if len(vals) == 0 {
		return ctx
	}
	ns, err := strconv.ParseInt(vals[0], 10, 64)
	if err != nil || ns <= 0 {
		return ctx
	}
	return WithMarks(ctx, time.Unix(0, ns), time.Time{}, time.Time{})
}

// WithMarks stores the clocks a session uses to split queue time.
// Zero values leave the previous clock in place.
func WithMarks(ctx context.Context, gateway, enqueued, loop time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	prev, _ := ctx.Value(marksKey{}).(marks)
	if !gateway.IsZero() {
		prev.gateway = gateway
	}
	if !enqueued.IsZero() {
		prev.enqueued = enqueued
	}
	if !loop.IsZero() {
		prev.loop = loop
	}
	return context.WithValue(ctx, marksKey{}, prev)
}

// Marks returns the clocks stored by WithMarks.
func Marks(ctx context.Context) (gateway, enqueued, loop time.Time) {
	if ctx == nil {
		return
	}
	m, _ := ctx.Value(marksKey{}).(marks)
	return m.gateway, m.enqueued, m.loop
}

// SessionDone logs one line for an RPC that has finished inside session.
// gw_ms is gateway receive to session enqueue. chan_ms is the session loop
// queue. rpcq_ms is the per-auth-key RPC queue, including a wait for a free
// worker. exec_ms is the bff/msg call itself.
func SessionDone(method string, authKey, msgID int64, gateway, enqueued, loop, queued, start, end time.Time, qlen int) {
	if !On() {
		return
	}
	logx.Infof("stage session method=%s auth_key=%d msg_id=%d gw_ms=%d chan_ms=%d rpcq_ms=%d exec_ms=%d qlen=%d",
		method, authKey, msgID,
		between(gateway, enqueued),
		between(enqueued, loop),
		between(queued, start),
		between(start, end),
		qlen)
}

func between(from, to time.Time) int64 {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return -1
	}
	return to.Sub(from).Milliseconds()
}
