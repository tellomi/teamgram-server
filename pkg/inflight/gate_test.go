package inflight

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTryAcquireShedsWhenFull(t *testing.T) {
	g := New(2)
	release1, ok := g.TryAcquire()
	if !ok {
		t.Fatal("first acquire")
	}
	release2, ok := g.TryAcquire()
	if !ok {
		t.Fatal("second acquire")
	}
	if _, ok := g.TryAcquire(); ok {
		t.Fatal("third acquire should shed")
	}
	release1()
	release3, ok := g.TryAcquire()
	if !ok {
		t.Fatal("acquire after release")
	}
	release2()
	release3()
	if _, ok := g.TryAcquire(); !ok {
		t.Fatal("acquire after all releases")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("TELLOMI_TEST_INFLIGHT", "")
	if got := FromEnv("TELLOMI_TEST_INFLIGHT", 256).Limit(); got != 256 {
		t.Fatalf("empty = %d", got)
	}
	t.Setenv("TELLOMI_TEST_INFLIGHT", "0")
	if got := FromEnv("TELLOMI_TEST_INFLIGHT", 256).Limit(); got != 256 {
		t.Fatalf("zero = %d", got)
	}
	t.Setenv("TELLOMI_TEST_INFLIGHT", "nope")
	if got := FromEnv("TELLOMI_TEST_INFLIGHT", 256).Limit(); got != 256 {
		t.Fatalf("invalid = %d", got)
	}
	t.Setenv("TELLOMI_TEST_INFLIGHT", "4")
	if got := FromEnv("TELLOMI_TEST_INFLIGHT", 256).Limit(); got != 4 {
		t.Fatalf("parsed = %d", got)
	}
}

func TestUnaryServerShedsAndRecovers(t *testing.T) {
	g := New(1)
	interceptor := UnaryServer(g)
	started := make(chan struct{})
	releaseHandler := make(chan struct{})
	var once sync.Once
	handler := func(context.Context, any) (any, error) {
		once.Do(func() { close(started) })
		<-releaseHandler
		return "ok", nil
	}
	errc := make(chan error, 1)
	go func() {
		_, err := interceptor(context.Background(), nil, nil, handler)
		errc <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	_, err := interceptor(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		t.Fatal("shed handler ran")
		return nil, nil
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("shed code %v", status.Code(err))
	}
	close(releaseHandler)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	got, err := interceptor(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		return "again", nil
	})
	if err != nil || got != "again" {
		t.Fatalf("recover got %v err %v", got, err)
	}
}

func TestUnaryServerNilGateDoesNotShed(t *testing.T) {
	got, err := UnaryServer(nil)(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	if err != nil || got != "ok" {
		t.Fatalf("got %v err %v", got, err)
	}
}

// Compile-time check that the interceptor matches the grpc type.
var _ grpc.UnaryServerInterceptor = UnaryServer(New(1))
