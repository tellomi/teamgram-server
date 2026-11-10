package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestSendFlightReturnsTheInFlightResult(t *testing.T) {
	f := newSendFlight()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var wg sync.WaitGroup
	var first, second *mtproto.Updates
	wg.Add(2)
	go func() {
		defer wg.Done()
		first, _ = f.Do(context.Background(), "7:99", func() (*mtproto.Updates, error) {
			calls.Add(1)
			close(started)
			<-release
			return mtproto.MakeTLUpdates(&mtproto.Updates{Seq: 3}).To_Updates(), nil
		})
	}()
	<-started
	go func() {
		defer wg.Done()
		second, _ = f.Do(context.Background(), "7:99", func() (*mtproto.Updates, error) {
			calls.Add(1)
			return mtproto.MakeTLUpdates(&mtproto.Updates{Seq: 9}).To_Updates(), nil
		})
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("calls %d", calls.Load())
	}
	if first.GetSeq() != 3 || second.GetSeq() != 3 {
		t.Fatalf("results %d %d", first.GetSeq(), second.GetSeq())
	}
}
