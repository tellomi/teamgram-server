package gnet

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"

	gnetlib "github.com/panjf2000/gnet/v2"
)

// addrHandler records LocalAddr of accepted connections. gnet sets that
// address from the listening socket (listeners[fd].addr), which is
// net.ResolveTCPAddr of the configured string, not getsockname.
type addrHandler struct {
	gnetlib.BuiltinEventEngine
	eng  atomic.Value // gnetlib.Engine
	seen chan string
}

func (h *addrHandler) OnBoot(eng gnetlib.Engine) (action gnetlib.Action) {
	h.eng.Store(eng)
	return gnetlib.None
}

func (h *addrHandler) OnOpen(c gnetlib.Conn) ([]byte, gnetlib.Action) {
	h.seen <- c.LocalAddr().String()
	return nil, gnetlib.Close
}

func TestAcceptedLocalAddrMatchesPPV1Config(t *testing.T) {
	plain := freePort(t)
	ppv1 := freePort(t)
	cfg := config.GnetwayConfig{
		Server: []config.GnetwayServer{
			{Proto: "tcp", Addresses: []string{"0.0.0.0:" + plain, "0.0.0.0:5222"}},
			{Proto: "websocket", Addresses: []string{"0.0.0.0:11443"}},
			{Proto: "tcp", PPV1: true, Addresses: []string{"0.0.0.0:" + ppv1}},
		},
	}
	if cfg.IsProxyProtocolV1("0.0.0.0:" + plain) {
		t.Fatal("plain listener matched PPV1")
	}
	if !cfg.IsProxyProtocolV1("0.0.0.0:" + ppv1) {
		t.Fatal("PPV1 listener did not match its own address")
	}
	// A dial to 127.0.0.1 must not be what the config compares.
	if cfg.IsProxyProtocolV1("127.0.0.1:" + ppv1) {
		t.Fatal("dial address matched PPV1; match is not by port")
	}

	h := &addrHandler{seen: make(chan string, 4)}
	errCh := make(chan error, 1)
	go func() {
		errCh <- gnetlib.Rotate(h, []string{
			"tcp://0.0.0.0:" + plain,
			"tcp://0.0.0.0:" + ppv1,
		}, gnetlib.WithMulticore(false), gnetlib.WithReuseAddr(true))
	}()
	t.Cleanup(func() {
		v := h.eng.Load()
		if v == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = v.(gnetlib.Engine).Stop(ctx)
	})

	waitListen(t, errCh, plain)
	waitListen(t, errCh, ppv1)
	dial(t, plain)
	dial(t, ppv1)

	got := map[string]bool{}
	deadline := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case addr := <-h.seen:
			got[addr] = true
		case err := <-errCh:
			t.Fatalf("gnet stopped: %v", err)
		case <-deadline:
			t.Fatalf("local addrs: %v", got)
		}
	}
	wantPlain := "0.0.0.0:" + plain
	wantPPV1 := "0.0.0.0:" + ppv1
	if !got[wantPlain] || !got[wantPPV1] {
		t.Fatalf("LocalAddr strings = %v, want %s and %s (not 127.0.0.1)", got, wantPlain, wantPPV1)
	}
	if cfg.IsProxyProtocolV1(wantPlain) {
		t.Fatalf("IsProxyProtocolV1(%s) = true", wantPlain)
	}
	if !cfg.IsProxyProtocolV1(wantPPV1) {
		t.Fatalf("IsProxyProtocolV1(%s) = false", wantPPV1)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitListen(t *testing.T, errCh <-chan error, port string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("gnet stopped before listen: %v", err)
		default:
		}
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		last = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("port %s did not accept: %v", port, last)
}

func dial(t *testing.T, port string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}
