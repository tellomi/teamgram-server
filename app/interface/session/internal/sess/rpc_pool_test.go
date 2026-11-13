package sess

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestSessionRPCConcurrencyEnv(t *testing.T) {
	t.Setenv("TELLOMI_SESSION_RPC_CONCURRENCY", "")
	if sessionRPCConcurrency() != defaultSessionRPCConcurrency {
		t.Fatalf("default = %d", sessionRPCConcurrency())
	}
	t.Setenv("TELLOMI_SESSION_RPC_CONCURRENCY", "1")
	if sessionRPCConcurrency() != 1 {
		t.Fatalf("serial override = %d", sessionRPCConcurrency())
	}
	t.Setenv("TELLOMI_SESSION_RPC_CONCURRENCY", "nope")
	if sessionRPCConcurrency() != defaultSessionRPCConcurrency {
		t.Fatal("bad value should keep the default")
	}
	t.Setenv("TELLOMI_SESSION_RPC_CONCURRENCY", "1000")
	if sessionRPCConcurrency() != maxSessionRPCConcurrency {
		t.Fatalf("cap = %d", sessionRPCConcurrency())
	}
}

func TestPooledRPCOverlaps(t *testing.T) {
	slots := make(chan struct{}, 2)
	var current, max atomic.Int32
	var wg sync.WaitGroup
	hold := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		startAuthRPC(slots, false, func() {
			defer wg.Done()
			n := current.Add(1)
			for {
				old := max.Load()
				if n <= old || max.CompareAndSwap(old, n) {
					break
				}
			}
			<-hold
			current.Add(-1)
		})
	}
	deadline := time.Now().Add(time.Second)
	for max.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if max.Load() < 2 {
		t.Fatal("two RPCs on a 2-slot pool did not overlap")
	}
	close(hold)
	wg.Wait()
}

func TestFileRPCDoesNotTakeSessionSlot(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	started := make(chan struct{})
	startAuthRPC(slots, true, func() { close(started) })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("file RPC waited for a session RPC slot")
	}
	if !isFileRPC(&mtproto.TLUploadGetFile{}) {
		t.Fatal("getFile must stay on the file pool")
	}
	if isFileRPC(&mtproto.TLMessagesSendMessage{}) {
		t.Fatal("sendMessage must use the session RPC pool")
	}
}

func TestSerialPoolDoesNotOverlap(t *testing.T) {
	slots := make(chan struct{}, 1)
	var current, max atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	startAuthRPC(slots, false, func() {
		defer wg.Done()
		current.Add(1)
		max.Add(1)
		<-release
		current.Add(-1)
	})
	time.Sleep(20 * time.Millisecond)
	secondStarted := make(chan struct{})
	go startAuthRPC(slots, false, func() { close(secondStarted) })
	select {
	case <-secondStarted:
		t.Fatal("second RPC ran while the only slot was held")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second RPC did not start after the slot was released")
	}
	if max.Load() != 1 {
		t.Fatalf("max in flight = %d", max.Load())
	}
}
