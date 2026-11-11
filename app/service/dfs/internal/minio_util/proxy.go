package minio_util

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// applyObjectHTTPProxy sends this transport's TCP connections through raw
// with HTTP CONNECT. An empty raw leaves the transport unchanged, so the
// endpoint is dialed directly. The process HTTP_PROXY environment is not
// read and http.DefaultTransport is not modified.
//
// The proxy is dialed instead of the origin. CONNECT failure and the
// connect deadline are returned to the caller. There is no direct-dial
// fallback. TLS and the signed Host header still name the origin: the
// proxy only sees the CONNECT target.
func applyObjectHTTPProxy(rt http.RoundTripper, raw string) (http.RoundTripper, error) {
	proxy, err := parseObjectProxy(raw)
	if err != nil || proxy == nil {
		return rt, err
	}
	base, ok := rt.(*http.Transport)
	if !ok || base == nil {
		return nil, fmt.Errorf("%s requires the object storage HTTP transport", EnvHTTPProxy)
	}
	dial := base.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	connectTimeout := base.TLSHandshakeTimeout
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}
	// Transport.Proxy would send cleartext requests to the proxy in
	// absolute form and would use a one-minute CONNECT deadline. Dialing
	// the proxy ourselves keeps one path for http and https and lets the
	// handshake timeout bound CONNECT.
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, proxy.Host)
		if err != nil {
			return nil, err
		}
		if err := writeConnect(conn, addr, proxy, connectTimeout); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
	return base, nil
}

func writeConnect(conn net.Conn, addr string, proxy *url.URL, timeout time.Duration) error {
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	defer conn.SetDeadline(time.Time{})
	var b strings.Builder
	fmt.Fprintf(&b, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	if proxy != nil && proxy.User != nil {
		user := proxy.User.Username()
		pass, _ := proxy.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		fmt.Fprintf(&b, "Proxy-Authorization: Basic %s\r\n", token)
	}
	b.WriteString("\r\n")
	if _, err := io.WriteString(conn, b.String()); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("object storage proxy CONNECT %s: %s", addr, resp.Status)
	}
	if br.Buffered() > 0 {
		return fmt.Errorf("object storage proxy CONNECT %s: unexpected bytes after status", addr)
	}
	return nil
}

func parseObjectProxy(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid %s %q", EnvHTTPProxy, strings.TrimSpace(raw))
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return nil, fmt.Errorf("invalid %s %q: scheme must be http or https", EnvHTTPProxy, strings.TrimSpace(raw))
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("invalid %s %q: port is required", EnvHTTPProxy, strings.TrimSpace(raw))
	}
	return u, nil
}

func redactProxy(raw string) string {
	u, err := parseObjectProxy(raw)
	if err != nil || u == nil || u.User == nil {
		return strings.TrimSpace(raw)
	}
	u.User = url.User("***")
	return u.String()
}
