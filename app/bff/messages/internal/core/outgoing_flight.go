package core

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/contextx"
)

// outgoingSendTimeout bounds a send whose client already disconnected.
// The retry waits on the same call, so the work must not inherit that
// client's cancel, and it must not run without a deadline.
const outgoingSendTimeout = 60 * time.Second

// outgoingFlight collapses concurrent sends that share a client random_id.
// sendMedia spends its time writing photo sizes before the message row
// exists. A client that times out and resends would otherwise run that work
// twice and, if both reached the insert, create two messages. The second
// call waits for the first and receives the same updates.
var outgoingFlight = newSendFlight()

type sendFlight struct {
	mu sync.Mutex
	m  map[string]*sendCall
}

type sendCall struct {
	done chan struct{}
	val  *mtproto.Updates
	err  error
}

func newSendFlight() *sendFlight {
	return &sendFlight{m: map[string]*sendCall{}}
}

func (f *sendFlight) Do(ctx context.Context, key string, fn func() (*mtproto.Updates, error)) (*mtproto.Updates, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	f.mu.Lock()
	if call, ok := f.m[key]; ok {
		f.mu.Unlock()
		select {
		case <-call.done:
			return call.val, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &sendCall{done: make(chan struct{})}
	f.m[key] = call
	f.mu.Unlock()

	call.val, call.err = fn()
	close(call.done)

	f.mu.Lock()
	delete(f.m, key)
	f.mu.Unlock()
	return call.val, call.err
}

// onceOutgoing runs fn once per (user, random_id) in this process.
// random_id 0 is not a client idempotency key. The leader's handler keeps
// running if the client drops the RPC, so the retry still observes the send.
func (c *MessagesCore) onceOutgoing(randomID int64, fn func() (*mtproto.Updates, error)) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || randomID == 0 {
		return fn()
	}
	key := strconv.FormatInt(c.MD.UserId, 10) + ":" + strconv.FormatInt(randomID, 10)
	return outgoingFlight.Do(c.ctx, key, func() (*mtproto.Updates, error) {
		return c.withDetachedSend(fn)
	})
}

// onceOutgoingIDs is onceOutgoing for an album. Every id must be non-zero.
func (c *MessagesCore) onceOutgoingIDs(ids []int64, fn func() (*mtproto.Updates, error)) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || len(ids) == 0 {
		return fn()
	}
	key := strconv.FormatInt(c.MD.UserId, 10)
	for _, id := range ids {
		if id == 0 {
			return fn()
		}
		key += ":" + strconv.FormatInt(id, 10)
	}
	return outgoingFlight.Do(c.ctx, key, func() (*mtproto.Updates, error) {
		return c.withDetachedSend(fn)
	})
}

func (c *MessagesCore) withDetachedSend(fn func() (*mtproto.Updates, error)) (*mtproto.Updates, error) {
	parent := c.ctx
	ctx, cancel := context.WithTimeout(contextx.ValueOnlyFrom(parent), outgoingSendTimeout)
	defer cancel()
	c.ctx = ctx
	defer func() { c.ctx = parent }()
	return fn()
}
