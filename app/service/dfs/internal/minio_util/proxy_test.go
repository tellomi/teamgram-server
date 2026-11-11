package minio_util

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestObjectProxyUnsetIgnoresProcessProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")
	t.Setenv(EnvHTTPProxy, "")

	before, beforeErr := defaultProxyResult(t)
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	srv, cfg := fake.server(t)
	defer srv.Close()
	if _, err := NewMinioClient(&cfg); err != nil {
		t.Fatalf("direct client with process HTTP_PROXY set: %v", err)
	}
	after, afterErr := defaultProxyResult(t)
	if before != after || (beforeErr == nil) != (afterErr == nil) {
		t.Fatalf("http.DefaultTransport proxy changed: %q/%v -> %q/%v", before, beforeErr, after, afterErr)
	}
	if cfg.HTTPProxy != "" {
		t.Fatalf("proxy %q", cfg.HTTPProxy)
	}
}

func TestApplyEnvParsesProxyHostPort(t *testing.T) {
	t.Setenv(EnvEndpoint, "https://account.r2.cloudflarestorage.com")
	t.Setenv(EnvAccessKeyID, "ak")
	t.Setenv(EnvSecretAccessKey, "sk")
	t.Setenv(EnvHTTPProxy, "172.19.44.222:3128")
	cfg := MinioConfig{}
	if err := cfg.ApplyEnv(); err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPProxy != "172.19.44.222:3128" {
		t.Fatalf("proxy %q", cfg.HTTPProxy)
	}
	u, err := parseObjectProxy(cfg.HTTPProxy)
	if err != nil || u.Scheme != "http" || u.Host != "172.19.44.222:3128" {
		t.Fatalf("parsed %#v %v", u, err)
	}
	secret := MinioConfig{HTTPProxy: "http://user:secret@172.19.44.222:3128"}
	got := secret.Redacted().HTTPProxy
	if strings.Contains(got, "secret") || strings.Contains(got, "user:") {
		t.Fatalf("proxy leaked credentials: %s", got)
	}
	if _, err = parseObjectProxy("ftp://172.19.44.222:3128"); err == nil {
		t.Fatal("ftp proxy accepted")
	}
}

func TestS3PutUsesConnectProxyNotDirect(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	rec := &headerRecorder{inner: fake}
	origin := httptest.NewTLSServer(rec)
	defer origin.Close()
	proxy, connects := connectProxy(t, "tunnel")
	defer proxy.Close()

	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	cfg := objectConfig(origin, tr)
	cfg.HTTPProxy = proxy.URL
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = client.PutPhotoFile(ctx, "m/1.dat", []byte("jpeg-thumb")); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetPhotoFile(ctx, "m/1.dat", 0, 32)
	if err != nil || string(got) != "jpeg-thumb" {
		t.Fatalf("get %q %v", got, err)
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
	if connects.Load() == 0 {
		t.Fatal("proxy saw no CONNECT")
	}
	host, auth := rec.snapshot()
	if host == "" || strings.Contains(host, proxy.Listener.Addr().String()) {
		t.Fatalf("origin host %q", host)
	}
	if !strings.Contains(auth, "AWS4-HMAC-SHA256") || !strings.Contains(strings.ToLower(auth), "host") {
		t.Fatalf("sigv4 %q", auth)
	}
}

func TestProxyRejectDoesNotDialDirect(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	defer origin.Close()
	proxy, connects := connectProxy(t, "reject")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	cfg := objectConfig(origin, tr)
	cfg.HTTPProxy = proxy.URL
	_, err := NewMinioClient(&cfg)
	if err == nil {
		t.Fatal("proxy reject succeeded")
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials after reject %d", direct.Load())
	}
	if connects.Load() == 0 {
		t.Fatal("reject was not sent to the proxy")
	}
	if strings.Contains(err.Error(), "direct dial") {
		t.Fatalf("fell back to direct: %v", err)
	}
}

func TestProxyHangTimesOutWithoutDirect(t *testing.T) {
	prev := minio.MaxRetry
	minio.MaxRetry = 1
	t.Cleanup(func() { minio.MaxRetry = prev })
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	defer origin.Close()
	proxy, _ := connectProxy(t, "hang")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 200*time.Millisecond)
	cfg := objectConfig(origin, tr)
	cfg.HTTPProxy = proxy.URL
	start := time.Now()
	_, err := NewMinioClient(&cfg)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("hung proxy succeeded")
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials after timeout %d", direct.Load())
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout took %s, err %v", elapsed, err)
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "timeout") && !strings.Contains(msg, "deadline") && !strings.Contains(msg, "handshake") {
		t.Fatalf("err %v", err)
	}
}

func defaultProxyResult(t *testing.T) (string, error) {
	t.Helper()
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		return "", errors.New("default transport has no proxy func")
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/object", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := tr.Proxy(req)
	if u == nil {
		return "", err
	}
	return u.String(), err
}

func objectConfig(origin *httptest.Server, tr *http.Transport) MinioConfig {
	cfg := MinioConfig{
		Endpoint:            origin.Listener.Addr().String(),
		AccessKeyID:         "minio",
		SecretAccessKey:     "miniostorage",
		UseSSL:              true,
		Region:              "auto",
		ForcePathStyle:      true,
		BucketProbeAttempts: 1,
		transport:           tr,
	}
	cfg.Bucket.Attachments = "tellomi-attachments"
	cfg.Bucket.PublicMedia = "tellomi-public-media"
	cfg.Bucket.Backups = "tellomi-backups"
	return cfg
}

func tlsTransport(t *testing.T, origin string, direct *atomic.Int32, handshake time.Duration) *http.Transport {
	t.Helper()
	tr, err := minio.DefaultTransport(true)
	if err != nil {
		t.Fatal(err)
	}
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	if handshake > 0 {
		tr.TLSHandshakeTimeout = handshake
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == origin {
			direct.Add(1)
			return nil, errors.New("direct dial " + addr)
		}
		d := net.Dialer{Timeout: time.Second}
		return d.DialContext(ctx, network, addr)
	}
	return tr
}

type headerRecorder struct {
	inner http.Handler
	host  atomic.Value
	auth  atomic.Value
}

func (h *headerRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.host.Store(r.Host)
	h.auth.Store(r.Header.Get("Authorization"))
	h.inner.ServeHTTP(w, r)
}

func (h *headerRecorder) snapshot() (host, auth string) {
	if v, ok := h.host.Load().(string); ok {
		host = v
	}
	if v, ok := h.auth.Load().(string); ok {
		auth = v
	}
	return host, auth
}

func connectProxy(t *testing.T, mode string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.Method != http.MethodConnect {
			http.Error(w, "only CONNECT", http.StatusMethodNotAllowed)
			return
		}
		switch mode {
		case "reject":
			http.Error(w, "proxy rejected", http.StatusForbidden)
			return
		case "hang":
			timer := time.NewTimer(10 * time.Second)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
			case <-timer.C:
			}
			return
		}
		dest, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer dest.Close()
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		done := make(chan struct{}, 2)
		go func() {
			_, _ = io.Copy(dest, conn)
			done <- struct{}{}
		}()
		go func() {
			_, _ = io.Copy(conn, dest)
			done <- struct{}{}
		}()
		<-done
	}))
	return srv, &n
}
