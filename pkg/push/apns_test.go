package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAPNSRequestShape(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	var gotHeader http.Header
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewAPNS(Config{
		KeyID:  "KEYID12345",
		TeamID: "TEAMID1234",
		Topic:  "app.tellomi.ios",
		KeyPEM: pemBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Point both hosts at the test server. host() prepends the scheme constant,
	// so replace the client transport by stubbing through a custom host via Sandbox false
	// and rewriting the request URL is not available. Call Send against a copied client
	// whose host function is bypassed by setting Sandbox and patching via a round tripper.
	client.httpClient = srv.Client()
	client.cfg.Sandbox = false
	payload := []byte(`{"aps":{"mutable-content":1}}`)

	// httptest is HTTP/1.1. Intercept by replacing the host lookup: Send uses cfg.host.
	// Swap in a round tripper that rewrites the Apple host to the test server.
	base := srv.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.httpClient.Transport = rewriteHost{base: base, dst: srv.URL}

	err = client.Send(context.Background(), "<ABCDEF 0123>", false, payload)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/3/device/abcdef0123" {
		t.Fatal(gotPath)
	}
	if gotHeader.Get("apns-topic") != "app.tellomi.ios" || gotHeader.Get("apns-push-type") != "alert" || gotHeader.Get("apns-priority") != "10" {
		t.Fatalf("%v", gotHeader)
	}
	auth := gotHeader.Get("authorization")
	if !strings.HasPrefix(auth, "bearer ") {
		t.Fatal(auth)
	}
	parts := strings.Split(strings.TrimPrefix(auth, "bearer "), ".")
	if len(parts) != 3 {
		t.Fatal(parts)
	}
	hdrJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var hdr map[string]string
	if err := json.Unmarshal(hdrJSON, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr["alg"] != "ES256" || hdr["kid"] != "KEYID12345" {
		t.Fatal(hdr)
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "TEAMID1234" {
		t.Fatal(claims)
	}
	if gotBody != string(payload) {
		t.Fatal(gotBody)
	}
	exp := gotHeader.Get("apns-expiration")
	if exp == "" || exp == "0" {
		t.Fatal("expiration must be in the future so offline devices are stored")
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"reason":"Unregistered"}`))
	}))
	defer srv2.Close()
	client.httpClient.Transport = rewriteHost{base: srv2.Client().Transport, dst: srv2.URL}
	err = client.Send(context.Background(), "aa", false, payload)
	ae, ok := err.(*APNSError)
	if !ok || !ae.DropDevice() || ae.Reason != "Unregistered" {
		t.Fatal(err)
	}
}

func TestAPNSVoIPRequestShape(t *testing.T) {
	client, srv, got := newAPNSCapture(t, "app.tellomi.chat")
	defer srv.Close()
	payload := []byte(`{"loc-key":"PHONE_CALL_REQUEST"}`)
	if err := client.SendVoIP(context.Background(), "<AABB>", false, payload); err != nil {
		t.Fatal(err)
	}
	if got.path != "/3/device/aabb" {
		t.Fatal(got.path)
	}
	if got.header.Get("apns-topic") != "app.tellomi.chat.voip" {
		t.Fatal(got.header.Get("apns-topic"))
	}
	if got.header.Get("apns-push-type") != "voip" || got.header.Get("apns-priority") != "10" || got.header.Get("apns-expiration") != "0" {
		t.Fatalf("%v", got.header)
	}
	if got.body != string(payload) {
		t.Fatal(got.body)
	}
}

func TestSandboxHost(t *testing.T) {
	c := Config{}
	if c.host(false) != productionHost || c.host(true) != sandboxHost {
		t.Fatal(c.host(false), c.host(true))
	}
	c.Sandbox = true
	if c.host(false) != sandboxHost {
		t.Fatal("forced sandbox")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv(EnvKeyID, "K")
	t.Setenv(EnvTeamID, "T")
	t.Setenv(EnvTopic, "app.tellomi.ios")
	t.Setenv(EnvKeyPath, "/keys/AuthKey.p8")
	t.Setenv(EnvSandbox, "false")
	t.Setenv(EnvVoIPTopic, "")
	cfg := LoadConfig()
	if !cfg.Enabled() || cfg.Sandbox || cfg.Topic != "app.tellomi.ios" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.ResolvedVoIPTopic() != "app.tellomi.ios.voip" {
		t.Fatal(cfg.ResolvedVoIPTopic())
	}
	t.Setenv(EnvVoIPTopic, "app.tellomi.chat.voip")
	if LoadConfig().ResolvedVoIPTopic() != "app.tellomi.chat.voip" {
		t.Fatal(LoadConfig().ResolvedVoIPTopic())
	}
	t.Setenv(EnvSandbox, "true")
	if !LoadConfig().Sandbox {
		t.Fatal("sandbox")
	}
}

type apnsCapture struct {
	path   string
	header http.Header
	body   string
}

func newAPNSCapture(t *testing.T, topic string) (*APNS, *httptest.Server, *apnsCapture) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	got := &apnsCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header = r.Header.Clone()
		got.path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	client, err := NewAPNS(Config{
		KeyID:  "KEYID12345",
		TeamID: "TEAMID1234",
		Topic:  topic,
		KeyPEM: pemBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = srv.Client()
	base := srv.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.httpClient.Transport = rewriteHost{base: base, dst: srv.URL}
	return client, srv, got
}

type rewriteHost struct {
	base http.RoundTripper
	dst  string
}

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(strings.TrimPrefix(r.dst, "https://"), "http://")
	req.Host = req.URL.Host
	if r.base == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return r.base.RoundTrip(req)
}

func TestSendAPNSLogsStatusAndKeyID(t *testing.T) {
	secret := bytes.Repeat([]byte{0x3c}, 256)
	sum := sha1.Sum(secret)
	wantID := hex.EncodeToString(sum[len(sum)-8:])
	msg := Incoming{
		UserID: 9, MsgID: 42, SenderID: 3, SenderName: "Ada",
		Text: "hello", LocKey: "MESSAGE_TEXT", PeerType: mtproto.PEER_USER, PeerID: 3,
	}
	prev := logx.Reset()
	sink := &infoLog{}
	logx.SetLevel(logx.InfoLevel)
	logx.SetWriter(sink)
	t.Cleanup(func() {
		logx.Reset()
		if prev != nil {
			logx.SetWriter(prev)
		}
	})

	dev := Device{UserID: 9, TokenType: TokenAPNS, Token: "deadbeefcafebabe", Secret: secret}
	svc := &Service{store: &MemoryStore{}, apns: nil, now: time.Now}

	t.Run("ok", func(t *testing.T) {
		sink.reset()
		client, srv, posted := apnsWithStatus(t, http.StatusOK, "")
		defer srv.Close()
		svc.apns = client
		svc.sendAPNS(context.Background(), dev, msg, Decision{Send: true, Preview: true})
		line := sink.find("push: apns")
		if line == "" {
			t.Fatalf("info lines %#v", sink.lines())
		}
		sentID, sentNext := keyIDAndNext(t, posted())
		want := fmt.Sprintf("push: apns user=%d msg=%d key_id=%s bytes=%d status=%d reason=", msg.UserID, msg.MsgID, sentID, len(posted()), http.StatusOK)
		if !strings.Contains(line, want) {
			t.Fatalf("line %q\nwant substring %q", line, want)
		}
		if sentID != wantID || strings.Contains(line, sentNext) || strings.Contains(line, hex.EncodeToString(secret)) || strings.Contains(line, dev.Token) {
			t.Fatalf("log included key material or the token: %s", line)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		sink.reset()
		client, srv, _ := apnsWithStatus(t, http.StatusBadRequest, "BadDeviceToken")
		defer srv.Close()
		svc.apns = client
		svc.sendAPNS(context.Background(), dev, msg, Decision{Send: true, Preview: true})
		line := sink.find("push: apns")
		want := fmt.Sprintf("status=%d reason=%s", http.StatusBadRequest, "BadDeviceToken")
		if !strings.Contains(line, want) || !strings.Contains(line, "key_id="+wantID) {
			t.Fatalf("line %q", line)
		}
		if strings.Contains(line, hex.EncodeToString(secret)) || strings.Contains(line, dev.Token) {
			t.Fatalf("log included key material or the token: %s", line)
		}
	})
}

func keyIDAndNext(t *testing.T, outer []byte) (string, string) {
	t.Helper()
	raw, err := decodeP(mustP(t, outer))
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw[:8]), hex.EncodeToString(raw[8:16])
}

func mustP(t *testing.T, outer []byte) string {
	t.Helper()
	var wrap struct {
		P string `json:"p"`
	}
	if err := json.Unmarshal(outer, &wrap); err != nil || wrap.P == "" {
		t.Fatal(err, wrap.P)
	}
	return wrap.P
}

func apnsWithStatus(t *testing.T, status int, reason string) (*APNS, *httptest.Server, func() []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	var posted []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		if reason != "" {
			_, _ = w.Write([]byte(`{"reason":"` + reason + `"}`))
		}
	}))
	client, err := NewAPNS(Config{
		KeyID: "KEYID12345", TeamID: "TEAMID1234", Topic: "app.tellomi.ios", KeyPEM: pemBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = srv.Client()
	base := srv.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.httpClient.Transport = rewriteHost{base: base, dst: srv.URL}
	return client, srv, func() []byte {
		return append([]byte(nil), posted...)
	}
}

type infoLog struct {
	mu   sync.Mutex
	info []string
}

func (s *infoLog) Alert(any)                   {}
func (s *infoLog) Close() error                { return nil }
func (s *infoLog) Debug(any, ...logx.LogField) {}
func (s *infoLog) Error(any, ...logx.LogField) {}
func (s *infoLog) Severe(any)                  {}
func (s *infoLog) Slow(any, ...logx.LogField)  {}
func (s *infoLog) Stack(any)                   {}
func (s *infoLog) Stat(any, ...logx.LogField)  {}
func (s *infoLog) Info(v any, _ ...logx.LogField) {
	s.mu.Lock()
	s.info = append(s.info, fmt.Sprint(v))
	s.mu.Unlock()
}

func (s *infoLog) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.info...)
}

func (s *infoLog) reset() {
	s.mu.Lock()
	s.info = nil
	s.mu.Unlock()
}

func (s *infoLog) find(substr string) string {
	for _, line := range s.lines() {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

func TestJWTCache(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewAPNS(Config{KeyID: "K", TeamID: "T", Topic: "topic", KeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})})
	if err != nil {
		t.Fatal(err)
	}
	a, err := client.bearer()
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.bearer()
	if err != nil || a != b {
		t.Fatal(err)
	}
	client.jwtIssued = time.Now().Add(-time.Hour)
	c, err := client.bearer()
	if err != nil || c == a {
		t.Fatal("expected a refreshed token")
	}
}
