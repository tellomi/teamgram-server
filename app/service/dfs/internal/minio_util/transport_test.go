package minio_util

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigureObjectTransportKeepsAlive(t *testing.T) {
	tr := &http.Transport{DisableKeepAlives: true, MaxIdleConns: 1, IdleConnTimeout: time.Second}
	configureObjectTransport(tr)
	if tr.DisableKeepAlives {
		t.Fatal("keep-alives should be on")
	}
	if tr.MaxIdleConns < r2MaxIdleConns || tr.MaxIdleConnsPerHost < r2MaxIdleConnsPerHost {
		t.Fatalf("pool caps: conns=%d perHost=%d", tr.MaxIdleConns, tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout < r2IdleConnTimeout {
		t.Fatalf("idle timeout %s", tr.IdleConnTimeout)
	}
	if tr.ResponseHeaderTimeout != r2ResponseHeaderTimeout || tr.TLSHandshakeTimeout != r2TLSHandshakeTimeout {
		t.Fatalf("timeouts header=%s tls=%s", tr.ResponseHeaderTimeout, tr.TLSHandshakeTimeout)
	}
}

func TestWarmupReusesConnections(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	srv, cfg := fake.server(t)
	t.Cleanup(srv.Close)

	var dials atomic.Int64
	tr := cfg.transport.(*http.Transport)
	prev := tr.DialContext
	if prev == nil {
		prev = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dials.Add(1)
		return prev(ctx, network, addr)
	}
	cfg.transport = tr

	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	started := dials.Load()
	if started == 0 {
		t.Fatal("probe/warmup should open at least one connection")
	}
	if _, err := client.client.BucketExists(context.Background(), "tellomi-attachments"); err != nil {
		t.Fatal(err)
	}
	if got := dials.Load(); got != started {
		t.Fatalf("expected connection reuse, dials after start=%d after another request=%d", started, got)
	}
}
