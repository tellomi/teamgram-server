package gnet

import (
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/inflight"
)

func TestBeginInflightShedsWithoutQueue(t *testing.T) {
	s := &Server{inflight: inflight.New(1)}
	release, ok := s.beginInflight()
	if !ok {
		t.Fatal("first")
	}
	if _, ok := s.beginInflight(); ok {
		t.Fatal("second should shed")
	}
	release()
	release2, ok := s.beginInflight()
	if !ok {
		t.Fatal("after release")
	}
	release2()
}

func TestBeginInflightNilGate(t *testing.T) {
	s := &Server{}
	var wg sync.WaitGroup
	wg.Add(8)
	for i := 0; i < 8; i++ {
		go func() {
			defer wg.Done()
			release, ok := s.beginInflight()
			if !ok {
				t.Error("nil gate shed")
			}
			release()
		}()
	}
	wg.Wait()
}
