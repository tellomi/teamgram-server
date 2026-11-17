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
	if sessionRPCConcurrency() != 1 || defaultSessionRPCConcurrency != 1 {
		t.Fatalf("default = %d", sessionRPCConcurrency())
	}
	t.Setenv("TELLOMI_SESSION_RPC_CONCURRENCY", "4")
	if sessionRPCConcurrency() != 4 {
		t.Fatalf("opt-in = %d", sessionRPCConcurrency())
	}
	t.Setenv("TELLOMI_SESSION_RPC_CONCURRENCY", "nope")
	if sessionRPCConcurrency() != 1 {
		t.Fatal("bad value should keep the serial default")
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

func TestSamePeerMutationsStayInOrder(t *testing.T) {
	orders := newPeerOrder()
	peer := inputPeerKey(&mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: 9})
	if peer != "user:9" {
		t.Fatal(peer)
	}
	var seq []int
	var mu sync.Mutex
	hold := make(chan struct{})
	wait1, release1 := orders.reserve(peer, false)
	wait2, release2 := orders.reserve(peer, false)
	other, otherAll, ordered := mutationOrder(&mtproto.TLUpdatesGetDifference{})
	if ordered || other != "" || otherAll {
		t.Fatal("getDifference must stay off the peer lane")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		wait1()
		mu.Lock()
		seq = append(seq, 1)
		mu.Unlock()
		<-hold
		release1()
	}()
	go func() {
		defer wg.Done()
		wait2()
		mu.Lock()
		seq = append(seq, 2)
		mu.Unlock()
		release2()
	}()
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	if len(seq) != 1 || seq[0] != 1 {
		mu.Unlock()
		t.Fatalf("second send started early: %v", seq)
	}
	mu.Unlock()
	close(hold)
	wg.Wait()
	if len(seq) != 2 || seq[0] != 1 || seq[1] != 2 {
		t.Fatalf("order = %v", seq)
	}
}

func TestDifferentPeersAndReadsOverlap(t *testing.T) {
	orders := newPeerOrder()
	a := inputPeerKey(&mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChat, ChatId: 3})
	b := inputPeerKey(&mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: 4})
	waitA, releaseA := orders.reserve(a, false)
	waitB, releaseB := orders.reserve(b, false)
	started := make(chan string, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		waitA()
		started <- "a"
		releaseA()
	}()
	go func() {
		defer wg.Done()
		waitB()
		started <- "b"
		releaseB()
	}()
	got := map[string]bool{}
	timeout := time.After(time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-timeout:
			t.Fatalf("different peers did not both start: %v", got)
		}
	}
	wg.Wait()
}

func TestDeleteMessagesBlocksEveryPeer(t *testing.T) {
	orders := newPeerOrder()
	sendPeer, all, ordered := mutationOrder(&mtproto.TLMessagesSendMessage{
		Peer: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: 8},
	})
	if !ordered || all || sendPeer != "user:8" {
		t.Fatalf("send peer=%s all=%v ordered=%v", sendPeer, all, ordered)
	}
	_, delAll, delOrdered := mutationOrder(&mtproto.TLMessagesDeleteMessages{Id: []int32{1}})
	if !delOrdered || !delAll {
		t.Fatal("deleteMessages must be a barrier")
	}
	fwd, _, fwdOrdered := mutationOrder(&mtproto.TLMessagesForwardMessages{
		ToPeer: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChat, ChatId: 2},
	})
	if !fwdOrdered || fwd != "chat:2" {
		t.Fatalf("forward lane = %s", fwd)
	}
	waitSend, releaseSend := orders.reserve(sendPeer, false)
	waitDel, releaseDel := orders.reserve("", true)
	waitEdit, releaseEdit := orders.reserve(sendPeer, false)
	var stage atomic.Int32
	go func() {
		waitSend()
		stage.Store(1)
		time.Sleep(40 * time.Millisecond)
		releaseSend()
	}()
	time.Sleep(10 * time.Millisecond)
	go func() {
		waitDel()
		stage.Store(2)
		releaseDel()
	}()
	time.Sleep(10 * time.Millisecond)
	if stage.Load() == 2 {
		t.Fatal("delete ran before the send released")
	}
	deadline := time.Now().Add(time.Second)
	for stage.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if stage.Load() != 2 {
		t.Fatal("delete did not run after the send")
	}
	editStarted := make(chan struct{})
	go func() {
		waitEdit()
		close(editStarted)
		releaseEdit()
	}()
	select {
	case <-editStarted:
	case <-time.After(time.Second):
		t.Fatal("edit stayed behind the delete barrier")
	}
}
