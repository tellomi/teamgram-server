package discoverdelete

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/pkg/discoverauth"
)

func TestMain(m *testing.M) {
	startBackground = false
	os.Exit(m.Run())
}

func TestCallMatchesEngineDeletion(t *testing.T) {
	key := bytesSeq(3, 32)
	pseudo := bytesSeq(4, 32)
	var gotAuth, gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvEngineURL, srv.URL)
	t.Setenv(EnvServiceKey, base64.StdEncoding.EncodeToString(key))
	t.Setenv(discoverauth.EnvPseudonymKey, base64.StdEncoding.EncodeToString(pseudo))
	cfg, err := load()
	if err != nil {
		t.Fatal(err)
	}
	sub := discoverauth.Pseudonym(pseudo, 42)
	result := Call(context.Background(), srv.Client(), cfg, sub)
	if result.Outcome != OutcomeDeleted || result.Status != http.StatusOK {
		t.Fatalf("%+v", result)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method %s", gotMethod)
	}
	wantPath := "/internal/users/" + discoverauth.EncodeSub(sub)
	if gotPath != wantPath {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer "+base64.StdEncoding.EncodeToString(key) {
		t.Fatal("authorization header mismatch")
	}
}

func TestClassify(t *testing.T) {
	if classify(200).Outcome != OutcomeDeleted || classify(503).Outcome != OutcomeRetry || classify(500).Outcome != OutcomeRetry || classify(401).Outcome != OutcomeAuth || classify(400).Outcome != OutcomePermanent || classify(404).Outcome != OutcomePermanent || classify(302).Outcome != OutcomePermanent {
		t.Fatal("classify")
	}
}

func TestRejectsPublicHTTP(t *testing.T) {
	t.Setenv(EnvEngineURL, "http://engine.example")
	t.Setenv(EnvServiceKey, base64.StdEncoding.EncodeToString(bytesSeq(3, 32)))
	t.Setenv(discoverauth.EnvPseudonymKey, base64.StdEncoding.EncodeToString(bytesSeq(4, 32)))
	if _, err := load(); err == nil {
		t.Fatal("http to a public host was accepted")
	}
}

func TestQueueRetriesThenSweeps(t *testing.T) {
	prev := secondDeleteAfter
	secondDeleteAfter = time.Hour
	t.Cleanup(func() { secondDeleteAfter = prev })
	resetQueue(t)
	key := bytesSeq(5, 32)
	pseudo := bytesSeq(6, 32)
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvEngineURL, "https://engine.example")
	t.Setenv(EnvServiceKey, base64.StdEncoding.EncodeToString(key))
	t.Setenv(discoverauth.EnvPseudonymKey, base64.StdEncoding.EncodeToString(pseudo))

	var statuses []int
	calls := 0
	Enqueue(42)
	qMu.Lock()
	qq := q
	qMu.Unlock()
	if qq == nil {
		t.Fatal("queue")
	}
	qq.do = func([]byte) Result {
		calls++
		if calls == 1 {
			return Result{Outcome: OutcomeRetry, Status: 503}
		}
		return Result{Outcome: OutcomeDeleted, Status: 200}
	}
	now := time.Now().Add(time.Minute)
	qq.mu.Lock()
	qq.items[0].next = now
	qq.mu.Unlock()
	qq.drain(now)
	qq.mu.Lock()
	if len(qq.items) != 1 || qq.items[0].attempts != 1 || !qq.items[0].next.After(now) {
		t.Fatalf("after retry %+v", qq.items)
	}
	qq.items[0].next = now
	qq.mu.Unlock()
	qq.drain(now)
	qq.mu.Lock()
	defer qq.mu.Unlock()
	if len(qq.items) != 1 || !qq.items[0].sweep || !qq.items[0].next.Equal(now.Add(time.Hour)) {
		t.Fatalf("sweep %+v", qq.items)
	}
	_ = statuses
}

func TestEnqueueOffDoesNothing(t *testing.T) {
	resetQueue(t)
	t.Setenv(EnvEnabled, "")
	Enqueue(7)
	qMu.Lock()
	defer qMu.Unlock()
	if q != nil {
		t.Fatal("queue created while off")
	}
}

func TestEnqueueDoesNotWaitOnTheEngine(t *testing.T) {
	resetQueue(t)
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvEngineURL, "https://engine.example")
	t.Setenv(EnvServiceKey, base64.StdEncoding.EncodeToString(bytesSeq(7, 32)))
	t.Setenv(discoverauth.EnvPseudonymKey, base64.StdEncoding.EncodeToString(bytesSeq(8, 32)))
	done := make(chan struct{})
	go func() {
		Enqueue(9)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked")
	}
}

func resetQueue(t *testing.T) {
	t.Helper()
	qMu.Lock()
	q = nil
	qMu.Unlock()
}

func bytesSeq(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}
