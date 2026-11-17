package sess

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

const (
	// defaultSessionRPCConcurrency stays 1. That is the old loop: one
	// non-file RPC per auth key, in the order the packets were queued.
	// Real clients do not pipeline several sends on one auth key, and that
	// total order is what invokeAfterMsg relies on (see below).
	// TELLOMI_SESSION_RPC_CONCURRENCY may raise the pool (max 32). A bad
	// value keeps 1. File RPCs never take a slot.
	defaultSessionRPCConcurrency = 1
	maxSessionRPCConcurrency     = 32
)

// invokeAfterMsg and invokeAfterMsgs are unwrapped in onInvokeAfterMsg /
// onInvokeAfterMsgs before the query is queued. The named msg_id is not
// waited on; the commented block in onInvokeAfterMsgs never shipped.
// With concurrency 1, execution order is queue order, so an inner query
// still runs after every RPC queued ahead of it. That is the only
// invokeAfter* barrier this process has.
//
// With concurrency above 1, reads may overlap. sendMessage, sendMedia,
// sendMultiMedia, forwardMessages, editMessage, and deleteHistory for the
// same peer still run one at a time, in arrival order, on that auth key.
// messages.deleteMessages carries no peer, so it is a barrier: it waits for
// mutations already reserved and later mutations wait for it. A forward is
// ordered on to_peer only. Two devices (two auth keys) do not share a lane.
func sessionRPCConcurrency() int {
	raw := os.Getenv("TELLOMI_SESSION_RPC_CONCURRENCY")
	n := defaultSessionRPCConcurrency
	if raw != "" {
		parsed, err := strconv.Atoi(raw)
		switch {
		case err != nil || parsed < 1:
			n = defaultSessionRPCConcurrency
		case parsed > maxSessionRPCConcurrency:
			n = maxSessionRPCConcurrency
		default:
			n = parsed
		}
	}
	logSessionRPCConcurrency(n)
	return n
}

// peerOrder keeps same-peer message mutations in the order reserve is
// called. The session loop calls reserve before starting the worker, so the
// chain follows packet order even when the workers themselves run together.
type peerOrder struct {
	mu      sync.Mutex
	pending []*peerTurn
}

type peerTurn struct {
	peer string
	all  bool
	done chan struct{}
}

func newPeerOrder() *peerOrder {
	return &peerOrder{}
}

// reserve records this mutation behind every earlier conflicting turn.
// wait blocks until those turns release. release must run once, after the
// RPC returns. all conflicts with every mutation (messages.deleteMessages,
// or a send whose peer could not be read).
func (o *peerOrder) reserve(peer string, all bool) (wait func(), release func()) {
	if o == nil {
		return func() {}, func() {}
	}
	t := &peerTurn{peer: peer, all: all, done: make(chan struct{})}
	o.mu.Lock()
	preds := make([]*peerTurn, 0, len(o.pending))
	for _, p := range o.pending {
		if all || p.all || (peer != "" && p.peer == peer) {
			preds = append(preds, p)
		}
	}
	o.pending = append(o.pending, t)
	o.mu.Unlock()
	var once sync.Once
	wait = func() {
		for _, p := range preds {
			<-p.done
		}
	}
	release = func() {
		once.Do(func() {
			close(t.done)
			o.mu.Lock()
			for i, p := range o.pending {
				if p == t {
					o.pending = append(o.pending[:i], o.pending[i+1:]...)
					break
				}
			}
			o.mu.Unlock()
		})
	}
	return wait, release
}

// mutationOrder reports whether msg must stay in order with other message
// mutations. peer is empty when the dialog is unknown; the caller then
// treats it as a barrier.
func mutationOrder(msg mtproto.TLObject) (peer string, all, ordered bool) {
	switch m := msg.(type) {
	case *mtproto.TLMessagesSendMessage:
		return inputPeerKey(m.GetPeer()), false, true
	case *mtproto.TLMessagesSendMedia:
		return inputPeerKey(m.GetPeer()), false, true
	case *mtproto.TLMessagesSendMultiMedia:
		return inputPeerKey(m.GetPeer()), false, true
	case *mtproto.TLMessagesForwardMessages:
		return inputPeerKey(m.GetToPeer()), false, true
	case *mtproto.TLMessagesEditMessage:
		return inputPeerKey(m.GetPeer()), false, true
	case *mtproto.TLMessagesDeleteHistory:
		return inputPeerKey(m.GetPeer()), false, true
	case *mtproto.TLMessagesDeleteMessages:
		return "", true, true
	default:
		return "", false, false
	}
}

func inputPeerKey(p *mtproto.InputPeer) string {
	if p == nil {
		return ""
	}
	switch p.GetPredicateName() {
	case mtproto.Predicate_inputPeerEmpty:
		return ""
	case mtproto.Predicate_inputPeerSelf:
		return "self"
	case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
		return fmt.Sprintf("user:%d", p.GetUserId())
	case mtproto.Predicate_inputPeerChat:
		return fmt.Sprintf("chat:%d", p.GetChatId())
	case mtproto.Predicate_inputPeerChannel, mtproto.Predicate_inputPeerChannelFromMessage:
		return fmt.Sprintf("channel:%d", p.GetChannelId())
	default:
		switch {
		case p.GetUserId() != 0:
			return fmt.Sprintf("user:%d", p.GetUserId())
		case p.GetChatId() != 0:
			return fmt.Sprintf("chat:%d", p.GetChatId())
		case p.GetChannelId() != 0:
			return fmt.Sprintf("channel:%d", p.GetChannelId())
		default:
			return ""
		}
	}
}

var sessionRPCLogOnce sync.Once

func logSessionRPCConcurrency(n int) {
	sessionRPCLogOnce.Do(func() {
		logx.Infof("session per-auth-key RPC concurrency is %d (TELLOMI_SESSION_RPC_CONCURRENCY)", n)
	})
}

// startAuthRPC runs run. A file RPC does not take a session slot, so a stuck
// send cannot block upload.getFile and a stuck file read cannot block sends.
// A non-file RPC takes one slot and holds it until run returns.
func startAuthRPC(slots chan struct{}, file bool, run func()) {
	if file {
		threading.GoSafe(run)
		return
	}
	slots <- struct{}{}
	threading.GoSafe(func() {
		defer func() { <-slots }()
		run()
	})
}
