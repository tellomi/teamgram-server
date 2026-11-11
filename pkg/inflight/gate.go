// Package inflight bounds work that is already running.
//
// TryAcquire fails immediately when the gate is full. Callers shed that
// request instead of parking it on an unbounded queue.
package inflight

import (
	"context"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Gate is a non-blocking counting semaphore.
type Gate struct {
	sem chan struct{}
}

// New returns a gate that allows n acquisitions at once.
// n below 1 is treated as 1 so a bad caller cannot open the gate.
func New(n int) *Gate {
	if n < 1 {
		n = 1
	}
	return &Gate{sem: make(chan struct{}, n)}
}

// Limit is the number of acquisitions the gate allows.
func (g *Gate) Limit() int {
	if g == nil || g.sem == nil {
		return 0
	}
	return cap(g.sem)
}

// TryAcquire takes one slot. The release function is always safe to call.
// ok is false when every slot is already held; release is then a no-op.
func (g *Gate) TryAcquire() (release func(), ok bool) {
	if g == nil || g.sem == nil {
		return func() {}, true
	}
	select {
	case g.sem <- struct{}{}:
		return func() { <-g.sem }, true
	default:
		return func() {}, false
	}
}

// FromEnv reads key as a positive integer. An empty or invalid value uses
// def, so a typo cannot drop the limit to zero or remove it.
func FromEnv(key string, def int) *Gate {
	n := def
	raw := strings.TrimSpace(os.Getenv(key))
	if raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed >= 1 {
			n = parsed
		}
	}
	return New(n)
}

// UnaryServer sheds an RPC with codes.ResourceExhausted when the gate is full.
// The handler is not called, so the rejected call does not occupy a worker
// beyond this check.
func UnaryServer(g *Gate) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		release, ok := g.TryAcquire()
		if !ok {
			return nil, status.Error(codes.ResourceExhausted, "overloaded")
		}
		defer release()
		return handler(ctx, req)
	}
}
