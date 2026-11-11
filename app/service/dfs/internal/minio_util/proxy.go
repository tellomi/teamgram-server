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
// with HTTP CONNECT. An empty raw dials endpoint directly and does not
// consult HTTP_PROXY or HTTPS_PROXY. http.DefaultTransport is not modified.
//
// The proxy is dialed instead of the origin, and only for that configured
// endpoint. Any other address is refused before a CONNECT is written.
// CONNECT failure and the connect deadline are returned to the caller.
// There is no direct-dial fallback. TLS and the signed Host header still
// name the origin: the proxy only sees the CONNECT target.
func applyObjectHTTPProxy(rt http.RoundTripper, raw, endpoint string, useSSL bool) (http.RoundTripper, error) {
	proxy, err := parseObjectProxy(raw)
	if err != nil {
		return nil, err
	}
	base, ok := rt.(*http.Transport)
	if !ok || base == nil {
		if proxy == nil {
			return rt, nil
		}
		return nil, fmt.Errorf("%s requires the object storage HTTP transport", EnvHTTPProxy)
	}
	// DefaultTransport.Proxy is ProxyFromEnvironment. Leaving it in place
	// would send a remote endpoint through the process proxy even when this
	// setting is empty. nil means dial the endpoint directly.
	base.Proxy = nil
	if proxy == nil {
		return base, nil
	}
	allowed := objectAddr(endpoint, useSSL)
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
	// the proxy ourselves keeps one path and lets the handshake timeout
	// bound CONNECT.
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !sameEndpoint(addr, allowed) {
			return nil, fmt.Errorf("object storage proxy refused unexpected target")
		}
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
		return nil, fmt.Errorf("invalid %s %s", EnvHTTPProxy, redactProxy(raw))
	}
	switch u.Scheme {
	case "http":
	case "https":
		return nil, fmt.Errorf("invalid %s %s: https proxies are refused so proxy credentials are not sent in cleartext; use http://host:port", EnvHTTPProxy, redactProxy(raw))
	default:
		return nil, fmt.Errorf("invalid %s %s: scheme must be http", EnvHTTPProxy, redactProxy(raw))
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("invalid %s %s: port is required", EnvHTTPProxy, redactProxy(raw))
	}
	return u, nil
}

func objectAddr(endpoint string, useSSL bool) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if strings.Contains(endpoint, "://") {
		if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
			endpoint = u.Host
		}
	}
	if _, _, err := net.SplitHostPort(endpoint); err == nil {
		return endpoint
	}
	if useSSL {
		return net.JoinHostPort(endpoint, "443")
	}
	return net.JoinHostPort(endpoint, "80")
}

func sameEndpoint(addr, allowed string) bool {
	if addr == "" || allowed == "" {
		return false
	}
	return addr == allowed
}

func redactProxy(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	candidate := raw
	if !strings.Contains(candidate, "://") {
		candidate = "http://" + candidate
	}
	if u, err := url.Parse(candidate); err == nil && u.User != nil {
		u.User = url.User("redacted")
		out := u.String()
		if !strings.Contains(raw, "://") {
			out = strings.TrimPrefix(out, "http://")
		}
		return out
	}
	return maskUserinfo(raw)
}

func maskUserinfo(raw string) string {
	prefix := ""
	rest := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		prefix = raw[:i+3]
		rest = raw[i+3:]
	}
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		return prefix + "redacted@" + rest[at+1:]
	}
	return raw
}
