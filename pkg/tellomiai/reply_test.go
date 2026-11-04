package tellomiai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type scripted struct {
	mu     sync.Mutex
	models []string
	mode   string
	auth   string
}

func (s *scripted) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.auth = r.Header.Get("Authorization")
	var body struct {
		Model string `json:"model"`
	}
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &body)
	s.models = append(s.models, body.Model)
	mode := s.mode
	s.mu.Unlock()
	if strings.Contains(s.auth, "secret-key") && strings.Contains(string(raw), "secret-key") {
		http.Error(w, "key echoed", http.StatusInternalServerError)
		return
	}
	switch mode {
	case "safety":
		if body.Model != defaultFallback {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Input data may contain inappropriate content.","type":"data_inspection_failed","code":"data_inspection_failed"}}`))
			return
		}
	case "native-safety":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"DataInspectionFailed","message":"Input data may contain inappropriate content.","request_id":"r"}`))
		return
	case "down":
		if body.Model == defaultPrimary {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"unavailable"}}`))
			return
		}
	case "slow":
		if body.Model == defaultPrimary {
			time.Sleep(200 * time.Millisecond)
		}
	case "slow-both":
		time.Sleep(200 * time.Millisecond)
	}
	_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}],"usage":{"total_tokens":100}}`))
}

func (s *scripted) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]string(nil), s.models...)
	return out
}

func testService(t *testing.T, mode string, timeout time.Duration) (*Service, *scripted, *httptest.Server) {
	t.Helper()
	sc := &scripted{mode: mode}
	srv := httptest.NewServer(http.HandlerFunc(sc.handler))
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL+"/v1", "secret-key", defaultPrimary, defaultSmall, defaultFallback, timeout)
	// NewClient appends /chat/completions onto BaseURL. The test server has no
	// prefix, so point BaseURL at the server origin's compatible path.
	client.BaseURL = srv.URL
	return &Service{enabled: true, Client: client, Ledger: NewLedger(), Now: func() time.Time {
		return time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)
	}}, sc, srv
}

func TestSafetyDoesNotCallDeepSeek(t *testing.T) {
	svc, sc, _ := testService(t, "safety", time.Second)
	res, err := svc.Reply(context.Background(), 9, "不行的问题", "e-safety")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Refused || !res.AIGenerated || res.Text != WithLabel(Refusal) {
		t.Fatalf("%#v", res)
	}
	if calls := sc.calls(); len(calls) != 1 || calls[0] != defaultPrimary {
		t.Fatalf("calls %v", calls)
	}
	if len(svc.Ledger.Records()) != 1 || !svc.Ledger.Records()[0].Refused {
		t.Fatal("refusal was not kept")
	}
}

func TestNativeSafetyCode(t *testing.T) {
	svc, sc, _ := testService(t, "native-safety", time.Second)
	res, err := svc.Reply(context.Background(), 9, "不行", "e-native")
	if err != nil || !res.Refused {
		t.Fatalf("%v %#v", err, res)
	}
	if len(sc.calls()) != 1 {
		t.Fatalf("fallback ran: %v", sc.calls())
	}
}

func TestPrimaryFailureUsesDeepSeekOnBailian(t *testing.T) {
	svc, sc, _ := testService(t, "down", time.Second)
	res, err := svc.Reply(context.Background(), 9, "你好", "e-fall")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != defaultFallback || !strings.HasSuffix(res.Text, Label) || !res.AIGenerated {
		t.Fatalf("%#v", res)
	}
	calls := sc.calls()
	if len(calls) != 2 || calls[0] != defaultPrimary || calls[1] != defaultFallback {
		t.Fatalf("%v", calls)
	}
	if sc.auth != "Bearer secret-key" {
		t.Fatalf("auth %q", sc.auth)
	}
}

func TestTimeoutThenFallback(t *testing.T) {
	svc, sc, _ := testService(t, "slow", 40*time.Millisecond)
	res, err := svc.Reply(context.Background(), 9, "你好", "e-time")
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != defaultFallback {
		t.Fatalf("model %s calls %v", res.Model, sc.calls())
	}
}

func TestBothCallsTimeOut(t *testing.T) {
	svc, _, _ := testService(t, "slow-both", 40*time.Millisecond)
	_, err := svc.Reply(context.Background(), 9, "你好", "e-both")
	if err != ErrTimeout {
		t.Fatalf("%v", err)
	}
}

func TestMinuteAndDailyLimits(t *testing.T) {
	svc, sc, _ := testService(t, "", time.Second)
	now := time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if _, err := svc.Reply(context.Background(), 9, "问", ""); err != nil {
			t.Fatal(err)
		}
	}
	res, err := svc.Reply(context.Background(), 9, "问", "")
	if err != nil || res.Limited != "minute" || res.Text != MinuteMessage || res.AIGenerated {
		t.Fatalf("%v %#v", err, res)
	}
	if len(sc.calls()) != 5 {
		t.Fatalf("minute still called the model: %d", len(sc.calls()))
	}

	svc.Ledger = NewLedger()
	svc.Now = func() time.Time { return now.Add(time.Duration(0) * time.Minute) }
	// Spread the 100 allowed calls across minutes so the minute cap does not trip.
	svc.Now = func() time.Time { return now }
	for i := 0; i < 100; i++ {
		minute := now.Add(time.Duration(i) * time.Minute)
		svc.Now = func() time.Time { return minute }
		if _, err := svc.Reply(context.Background(), 9, "问", ""); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	svc.Now = func() time.Time { return now.Add(100 * time.Minute) }
	res, err = svc.Reply(context.Background(), 9, "问", "")
	if err != nil || res.Limited != "daily" || res.Text != DailyMessage {
		t.Fatalf("%v %#v", err, res)
	}
}

func TestBudgetSmallThenPause(t *testing.T) {
	svc, sc, _ := testService(t, "", time.Second)
	svc.Ledger.MonthBudget = 10
	svc.Ledger.PricePerToken = 0.08
	first, err := svc.Reply(context.Background(), 9, "问", "")
	if err != nil || first.Model != defaultPrimary {
		t.Fatalf("%v %#v", err, first)
	}
	second, err := svc.Reply(context.Background(), 9, "问", "")
	if err != nil || second.Model != defaultSmall {
		t.Fatalf("%v %#v calls %v", err, second, sc.calls())
	}
	third, err := svc.Reply(context.Background(), 9, "问", "")
	if err != nil || third.Limited != "budget" || third.Text != BudgetMessage || third.AIGenerated {
		t.Fatalf("%v %#v", err, third)
	}
	if len(sc.calls()) != 2 {
		t.Fatalf("paused call still hit the model: %v", sc.calls())
	}
}

func TestPrivateReplyOnce(t *testing.T) {
	svc, sc, _ := testService(t, "", time.Second)
	sent := &memSend{}
	svc.Sender = sent
	Use(svc)
	t.Cleanup(func() { Use(nil) })
	ObservePrivate(context.Background(), 9, "你好", 99)
	ObservePrivate(context.Background(), 9, "你好", 99)
	ObservePrivate(context.Background(), AccountID, "你好", 1)
	if sent.n != 1 || !sent.ai || !strings.HasSuffix(sent.text, Label) || !strings.Contains(sent.text, "你好") {
		t.Fatalf("sent n=%d ai=%v text=%q", sent.n, sent.ai, sent.text)
	}
	if len(sc.calls()) != 1 {
		t.Fatalf("calls %v", sc.calls())
	}
}

func TestDisabledDoesNotCallModel(t *testing.T) {
	sc := &scripted{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("model called while disabled")
	}))
	defer srv.Close()
	_ = sc
	Use(&Service{enabled: false, Client: NewClient(srv.URL, "secret-key", "", "", "", time.Second)})
	t.Cleanup(func() { Use(nil) })
	ObservePrivate(context.Background(), 9, "你好", 1)
	if Advertised() {
		t.Fatal("advertised without the flag")
	}
}

type memSend struct {
	n    int
	text string
	ai   bool
}

func (m *memSend) Send(_ context.Context, _ int64, text string, ai bool) error {
	m.n++
	m.text = text
	m.ai = ai
	return nil
}

type memSubs struct {
	sub string
	id  int64
}

func (m memSubs) Users(_ context.Context, subs [][]byte) (map[string]int64, error) {
	out := map[string]int64{}
	for _, sub := range subs {
		if base64.RawURLEncoding.EncodeToString(sub) == m.sub {
			out[m.sub] = m.id
		}
	}
	return out, nil
}

func TestEngineCompleteE2E(t *testing.T) {
	svc, sc, _ := testService(t, "", time.Second)
	rawSub := []byte("0123456789abcdef")
	enc := base64.RawURLEncoding.EncodeToString(rawSub)
	key := []byte("0123456789abcdef0123456789abcdef")
	h := &Handler{Service: svc, Subjects: memSubs{sub: enc, id: 9}, Key: key}
	srv := httptest.NewServer(h)
	defer srv.Close()
	post := func(raw string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, srv.URL+pathComplete, strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(key))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	body := `{"event_id":"mention-1","user_sub":"` + enc + `","text":"@tellomi 你好"}`
	resp := post(body)
	defer resp.Body.Close()
	var res Result
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !res.AIGenerated || !strings.HasSuffix(res.Text, Label) {
		t.Fatalf("status %d %#v", resp.StatusCode, res)
	}
	resp2 := post(body)
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("retry status %d", resp2.StatusCode)
	}
	if len(sc.calls()) != 1 {
		t.Fatalf("engine retry called the model again: %v", sc.calls())
	}

	svc2, _, _ := testService(t, "slow-both", 30*time.Millisecond)
	h.Service = svc2
	resp3 := post(`{"event_id":"mention-2","user_sub":"` + enc + `","text":"慢"}`)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("timeout status %d", resp3.StatusCode)
	}
}
