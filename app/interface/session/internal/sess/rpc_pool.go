package sess

import (
	"os"
	"strconv"
	"sync"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

const (
	defaultSessionRPCConcurrency = 4
	maxSessionRPCConcurrency     = 32
)

// sessionRPCConcurrency is how many non-file RPCs one auth key may run at
// once. The historical loop ran them one after another, so a 200ms send
// capped that device near 5 QPS and a retry parked behind it. File RPCs keep
// their own pool and do not take one of these slots.
// TELLOMI_SESSION_RPC_CONCURRENCY=1 restores the serial loop. A bad value
// keeps the default. Values above 32 are capped.
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
