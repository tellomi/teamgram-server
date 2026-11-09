package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

func TestSendRunsDevicesInBoundedPoolNewestFirst(t *testing.T) {
	const n = 6
	base := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	store := &MemoryStore{}
	ctx := context.Background()
	newest := make([]string, 0, sendConcurrency)
	var all []string
	for i := 1; i <= n; i++ {
		token := fmt.Sprintf("tok%d", i)
		all = append(all, token)
		if i > n-sendConcurrency {
			newest = append(newest, token)
		}
		if err := store.Upsert(ctx, Device{
			AuthKeyID: int64(i),
			UserID:    9,
			TokenType: TokenAPNS,
			Token:     token,
			Secret:    bytesRepeat(256),
			UpdatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Token != fmt.Sprintf("tok%d", n) {
		t.Fatalf("newest token %s, want tok%d", listed[0].Token, n)
	}

	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var inflight, maxIn int
	var order []string
	var firstWave []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/3/device/")
		var open bool
		mu.Lock()
		inflight++
		if inflight > maxIn {
			maxIn = inflight
		}
		order = append(order, token)
		if len(order) == sendConcurrency && firstWave == nil {
			firstWave = append([]string(nil), order...)
			open = true
		}
		mu.Unlock()
		if open {
			once.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		mu.Lock()
		inflight--
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewAPNS(Config{
		KeyID: "KEYID12345", TeamID: "TEAMID1234", Topic: "app.tellomi.chat",
		KeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
	})
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = srv.Client()
	baseTransport := srv.Client().Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	client.httpClient.Transport = rewriteHost{base: baseTransport, dst: srv.URL}

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

	svc := NewService(store, Config{})
	svc.apns = client
	svc.Deliver(ctx, 9, textUpdate())

	mu.Lock()
	gotMax := maxIn
	wave := append([]string(nil), firstWave...)
	mu.Unlock()
	if gotMax != sendConcurrency {
		t.Fatalf("in flight %d, want the pool cap %d", gotMax, sendConcurrency)
	}
	if len(wave) != sendConcurrency {
		t.Fatalf("first wave %v", wave)
	}
	want := map[string]bool{}
	for _, token := range newest {
		want[token] = true
	}
	for _, token := range wave {
		if !want[token] {
			t.Fatalf("first wave %v, want the newest %v", wave, newest)
		}
		delete(want, token)
	}
	if len(want) != 0 {
		t.Fatalf("first wave missed %v (wave %v)", want, wave)
	}

	lines := sink.lines()
	logged := 0
	for _, line := range lines {
		if strings.Contains(line, "push: apns user=9 msg=4 ") && strings.Contains(line, "status=200") {
			logged++
		}
		for _, token := range all {
			if strings.Contains(line, token) {
				t.Fatalf("log included a device token: %s", line)
			}
		}
	}
	if logged != n {
		t.Fatalf("apns log lines %d, want %d; lines %#v", logged, n, lines)
	}
}

func TestSortDevicesNewestFirst(t *testing.T) {
	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	in := []Device{
		{Token: "old", UpdatedAt: base},
		{Token: "none"},
		{Token: "new", UpdatedAt: base.Add(time.Hour)},
		{Token: "mid", UpdatedAt: base.Add(time.Minute)},
	}
	sortDevicesNewestFirst(in)
	got := []string{in[0].Token, in[1].Token, in[2].Token, in[3].Token}
	want := []string{"new", "mid", "old", "none"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}
