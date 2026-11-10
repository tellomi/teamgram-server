//go:build reliability

package e2e

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// s3Proxy is the object-store address dfs dials. It forwards to the
// MinIO-compatible container unless a test asks for HTTP 500 or a hung PUT.
// HEAD and GET always pass so bucket checks and downloads are not faults.
type s3Proxy struct {
	target string

	mu   sync.Mutex
	mode int32
	hold chan struct{}
	puts atomic.Int32

	srv  *http.Server
	port int
}

const (
	s3Pass int32 = 0
	s3Fail int32 = 1
	s3Hold int32 = 2
)

func newS3Proxy(backendHostPort string) (*s3Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &s3Proxy{
		target: backendHostPort,
		hold:   make(chan struct{}),
		port:   ln.Addr().(*net.TCPAddr).Port,
	}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

func (p *s3Proxy) Port() int { return p.port }

func (p *s3Proxy) Close() {
	if p == nil || p.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.srv.Shutdown(ctx)
}

func (p *s3Proxy) setMode(mode int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mode == s3Hold && mode != s3Hold {
		close(p.hold)
		p.hold = make(chan struct{})
	}
	p.mode = mode
}

func (p *s3Proxy) resetPuts() { p.puts.Store(0) }

func (p *s3Proxy) waitPut(ctx context.Context) error {
	for {
		if p.puts.Load() > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (p *s3Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		p.mu.Lock()
		mode := p.mode
		hold := p.hold
		p.mu.Unlock()
		switch mode {
		case s3Fail:
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			p.puts.Add(1)
			http.Error(w, "injected storage failure", http.StatusInternalServerError)
			return
		case s3Hold:
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			p.puts.Add(1)
			select {
			case <-hold:
			case <-r.Context().Done():
			}
			http.Error(w, "upload interrupted", http.StatusBadGateway)
			return
		}
	}
	if err := p.forward(w, r); err != nil {
		http.Error(w, "object store proxy: "+err.Error(), http.StatusBadGateway)
	}
}

func (p *s3Proxy) forward(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	_ = r.Body.Close()
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://"+p.target+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	// The client signed this Host. Replacing it with the container address
	// makes the object store reject the signature.
	req.Host = r.Host
	req.ContentLength = int64(len(body))
	for k, vs := range r.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Host", "Content-Length", "Transfer-Encoding", "Connection":
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}
