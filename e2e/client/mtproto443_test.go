package e2e

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMtproto443Stream runs nginx:1.24.0 (stream + ssl_preread) in front of
// the compose gnetway using deploy/nginx/chengdu-stream.conf. The MTProto
// dials are IP literals and are not TLS, so they send no SNI. Routing is
// the empty ssl_preread protocol, which is what the Android client needs
// when it dials the raw IP on 443.
func TestMtproto443Stream(t *testing.T) {
	if os.Getenv("TELLOMI_E2E") == "" {
		t.Skip("set TELLOMI_E2E=1 to run against a Teamgram stack")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("docker is required")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatal("python3 is required")
	}
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Fatal("openssl is required")
	}
	root := e2eRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := waitTCP(ctx, "127.0.0.1:10443"); err != nil {
		t.Fatalf("gnetway 10443: %v", err)
	}
	if err := waitTCP(ctx, "127.0.0.1:10444"); err != nil {
		t.Fatalf("gnetway 10444: %v", err)
	}

	dir := t.TempDir()
	wwwCert, wwwKey := writeCert(t, dir, "www", "www.tellomi.cn", "www.tellomi.cn", "api.tellomi.cn")
	grpcCert, grpcKey := writeCert(t, dir, "grpc", "grpc.chat.tellomi.cn", "grpc.chat.tellomi.cn")
	pool := x509.NewCertPool()
	for _, path := range []string{wwwCert, grpcCert} {
		pem, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			t.Fatalf("cert pool rejected %s", path)
		}
	}
	serveHTTPS(t, "127.0.0.1:4443", wwwCert, wwwKey, "tellomi-www-h2")
	serveHTTPS(t, "127.0.0.1:8443", grpcCert, grpcKey, "tellomi-grpc-h2")

	stream := adaptStream(t, root, dir)
	name := startNginx(t, dir, stream)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer waitCancel()
	if err := waitTCP(waitCtx, "127.0.0.1:18443"); err != nil {
		out, _ := exec.Command("docker", "logs", name).CombinedOutput()
		t.Fatalf("nginx 18443: %v\n%s", err, out)
	}

	// (a) obfuscated abridged through the 443-equivalent port, by IP, no SNI.
	through := runProbe(t, root, "127.0.0.1", "18443", "--avoid-preread-wait")
	mustResPQ(t, through)

	// (b) PROXY v1 source is the address gnetway records, not the socket peer.
	forged := runProbe(t, root, "127.0.0.1", "10444", "--proxy-v1-src", "203.0.113.77", "--avoid-preread-wait")
	mustResPQ(t, forged)
	waitGnetwayLog(t, "ppv1 accepted client_ip=203.0.113.77 local=0.0.0.0:10444")
	waitGnetwayLog(t, "ppv1 accepted client_ip=127.0.0.1 local=0.0.0.0:10444")

	// (c) TLS ClientHello with SNI still reaches the matching HTTPS backend.
	getH2(t, pool, "www.tellomi.cn", "tellomi-www-h2")
	getH2(t, pool, "grpc.chat.tellomi.cn", "tellomi-grpc-h2")

	// (d) direct 10443 still handshakes, and a PROXY header there is not trusted.
	direct := runProbe(t, root, "127.0.0.1", "10443", "--avoid-preread-wait")
	mustResPQ(t, direct)
	untrusted := runProbe(t, root, "127.0.0.1", "10443", "--proxy-v1-src", "203.0.113.9", "--timeout", "3", "--avoid-preread-wait")
	if ok, _ := untrusted["ok"].(bool); ok {
		t.Fatalf("10443 trusted a PROXY header: %s", mustJSON(untrusted))
	}
	time.Sleep(500 * time.Millisecond)
	if logHas(t, "client_ip=203.0.113.9") {
		t.Fatal("10443 logged a PROXY header source address")
	}

	// 16 03 .. makes ssl_preread wait, then nginx 1.24 closes. gnetway would
	// have answered resPQ if the bytes had been proxied.
	edge := runProbe(t, root, "127.0.0.1", "18443", "--init-prefix", "160300ffff", "--timeout", "8")
	if ok, _ := edge["ok"].(bool); ok {
		t.Fatalf("TLS-lookalike init was proxied: %s", mustJSON(edge))
	}
	ms, _ := edge["elapsed_ms"].(float64)
	if ms < 3500 || ms > 9000 {
		t.Fatalf("preread timeout was not about 5s: %s", mustJSON(edge))
	}
}

func e2eRoot(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("TELLOMI_E2E_ROOT"); dir != "" {
		return dir
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func writeCert(t *testing.T, dir, name, cn string, dnsNames ...string) (cert, key string) {
	t.Helper()
	cert = filepath.Join(dir, name+".crt")
	key = filepath.Join(dir, name+".key")
	san := "subjectAltName=DNS:" + strings.Join(dnsNames, ",DNS:")
	cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
		"-keyout", key, "-out", cert, "-days", "1",
		"-subj", "/CN="+cn, "-addext", san)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("openssl req: %v\n%s", err, out)
	}
	return cert, key
}

func serveHTTPS(t *testing.T, addr, cert, key, marker string) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, marker)
		}),
	}
	go func() { _ = srv.ServeTLS(&proxyListener{Listener: ln}, cert, key) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitTCP(ctx, addr); err != nil {
		t.Fatal(err)
	}
}

type proxyListener struct{ net.Listener }

func (l *proxyListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		br := bufio.NewReader(c)
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		line, err := br.ReadString('\n')
		_ = c.SetReadDeadline(time.Time{})
		if err != nil || !strings.HasPrefix(line, "PROXY ") || !strings.HasSuffix(line, "\r\n") {
			_ = c.Close()
			continue
		}
		return &prefixConn{Conn: c, r: br}, nil
	}
}

type prefixConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *prefixConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func adaptStream(t *testing.T, root, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "deploy", "nginx", "chengdu-stream.conf"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, needle := range []string{
		"grpc.chat.tellomi.cn 127.0.0.1:8443;",
		"default              127.0.0.1:4443;",
		`""      127.0.0.1:10444;`,
		"preread_timeout 5s;",
		"proxy_protocol on;",
		"listen 443;",
		"listen [::]:443;",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("template missing %q", needle)
		}
	}
	text = strings.ReplaceAll(text, "listen 443;", "listen 127.0.0.1:18443;")
	text = strings.ReplaceAll(text, "listen [::]:443;", "listen [::1]:18443;")
	path := filepath.Join(dir, "chengdu-stream.conf")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func startNginx(t *testing.T, dir, stream string) string {
	t.Helper()
	name := fmt.Sprintf("mtproto443-nginx-%d", os.Getpid())
	modules := []string{
		"/usr/lib/nginx/modules/ngx_stream_module.so",
		"/etc/nginx/modules/ngx_stream_module.so",
	}
	var last []byte
	for _, mod := range modules {
		conf := filepath.Join(dir, "nginx.conf")
		body := fmt.Sprintf(`load_module %s;
worker_processes 1;
error_log /dev/stderr info;
pid /tmp/nginx-mtproto443.pid;
events { worker_connections 128; }
stream {
    include /etc/nginx/chengdu-stream.conf;
}
`, mod)
		if err := os.WriteFile(conf, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		test := exec.Command("docker", "run", "--rm", "--network", "host", "--entrypoint", "nginx",
			"-v", conf+":/etc/nginx/nginx.conf:ro",
			"-v", stream+":/etc/nginx/chengdu-stream.conf:ro",
			"nginx:1.24.0", "-t")
		out, err := test.CombinedOutput()
		last = out
		if err != nil {
			continue
		}
		run := exec.Command("docker", "run", "-d", "--name", name, "--network", "host", "--entrypoint", "nginx",
			"-v", conf+":/etc/nginx/nginx.conf:ro",
			"-v", stream+":/etc/nginx/chengdu-stream.conf:ro",
			"nginx:1.24.0", "-g", "daemon off;")
		out, err = run.CombinedOutput()
		if err != nil {
			t.Fatalf("nginx start: %v\n%s", err, out)
		}
		return name
	}
	t.Fatalf("nginx -t failed for stream module paths\n%s", last)
	return ""
}

func runProbe(t *testing.T, root string, args ...string) map[string]any {
	t.Helper()
	cmdArgs := append([]string{filepath.Join(root, "scripts", "mtproto-probe.py")}, args...)
	cmd := exec.Command("python3", cmdArgs...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	line := strings.TrimSpace(string(out))
	t.Logf("MTPROTO443_RESULT %s", line)
	var doc map[string]any
	if jerr := json.Unmarshal([]byte(lastJSONLine(line)), &doc); jerr != nil {
		t.Fatalf("probe: %v\n%s", err, line)
	}
	return doc
}

func lastJSONLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "{") {
			return trim
		}
	}
	return strings.TrimSpace(s)
}

func mustResPQ(t *testing.T, doc map[string]any) {
	t.Helper()
	if ok, _ := doc["ok"].(bool); !ok {
		t.Fatalf("handshake failed: %s", mustJSON(doc))
	}
	if doc["request"] != "req_pq_multi#be7e8ef1" || doc["response"] != "resPQ#05162463" || doc["nonce_match"] != true {
		t.Fatalf("unexpected handshake summary: %s", mustJSON(doc))
	}
	if doc["sni"] != "" {
		t.Fatalf("probe reported an SNI: %s", mustJSON(doc))
	}
}

func mustJSON(doc map[string]any) string {
	b, err := json.Marshal(doc)
	if err != nil {
		return fmt.Sprint(doc)
	}
	return string(b)
}

func getH2(t *testing.T, pool *x509.CertPool, name, marker string) {
	t.Helper()
	tlsCfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
	}
	tr := &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   tlsCfg,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			cfg := tlsCfg.Clone()
			cfg.ServerName = host
			d := &tls.Dialer{Config: cfg}
			return d.DialContext(ctx, "tcp", "127.0.0.1:18443")
		},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get("https://" + name + "/")
	if err != nil {
		t.Fatalf("https %s: %v", name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(body) != marker {
		t.Fatalf("https %s status=%d body=%q", name, resp.StatusCode, body)
	}
	if resp.Proto != "HTTP/2.0" || resp.TLS == nil || resp.TLS.NegotiatedProtocol != "h2" {
		t.Fatalf("https %s proto=%s alpn=%v", name, resp.Proto, resp.TLS)
	}
	if len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("no peer certificate")
	}
	if err := resp.TLS.PeerCertificates[0].VerifyHostname(name); err != nil {
		t.Fatalf("cert for %s: %v", name, err)
	}
	t.Logf("MTPROTO443_RESULT https name=%s proto=%s alpn=%s status=%d body=%s sni=%s",
		name, resp.Proto, resp.TLS.NegotiatedProtocol, resp.StatusCode, marker, name)
}

func waitGnetwayLog(t *testing.T, needle string) {
	t.Helper()
	var last string
	for i := 0; i < 20; i++ {
		out, err := gnetwayLog(needle)
		last = out
		if err == nil && strings.Contains(out, needle) {
			t.Logf("MTPROTO443_RESULT log %s", needle)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	listing, _ := composeExec(15*time.Second, "", "sh", "-c", "ls -la /app/logs/gnetway || true")
	t.Fatalf("gnetway log missing %q\n%s\n%s", needle, last, listing)
}

func logHas(t *testing.T, needle string) bool {
	t.Helper()
	out, err := gnetwayLog(needle)
	return err == nil && strings.Contains(out, needle)
}

func gnetwayLog(needle string) (string, error) {
	script := "grep -R -F -- " + quoteSh(needle) + " /app/logs/gnetway || true"
	return composeExec(15*time.Second, "", "sh", "-c", script)
}

func quoteSh(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
