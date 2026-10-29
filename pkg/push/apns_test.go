package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	cfg := LoadConfig()
	if !cfg.Enabled() || cfg.Sandbox || cfg.Topic != "app.tellomi.ios" {
		t.Fatalf("%+v", cfg)
	}
	t.Setenv(EnvSandbox, "true")
	if !LoadConfig().Sandbox {
		t.Fatal("sandbox")
	}
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
