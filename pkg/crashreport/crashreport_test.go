package crashreport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestObjectPathAndHints(t *testing.T) {
	at := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	got := ObjectPath(at, 42, 7, `../Crash Log iOS.zip`)
	if got != "2026/11/10/42/7_Crash_Log_iOS.zip" {
		t.Fatalf("path %s", got)
	}
	if FullKey(got) != "crash-reports/2026/11/10/42/7_Crash_Log_iOS.zip" {
		t.Fatalf("key %s", FullKey(got))
	}
	if PathOf(FullKey(got)) != got {
		t.Fatalf("strip %s", PathOf(FullKey(got)))
	}
	device, app := Hints("Tellomi-iOS-crash.zip", "app=14.0.3 device=iPhone15,2", "")
	if !strings.EqualFold(device, "iOS") && !strings.Contains(strings.ToLower(device), "iphone") {
		t.Fatalf("device %q", device)
	}
	if app != "14.0.3" {
		t.Fatalf("app %q", app)
	}
	if d, a := Hints("note.txt", "hello", ""); d != "" || a != "" {
		t.Fatalf("plain hints %q %q", d, a)
	}
}

func TestUsernameDefault(t *testing.T) {
	t.Cleanup(func() { published.Store("") })
	t.Setenv(EnvBot, "")
	if Username() != DefaultUsername {
		t.Fatal(Username())
	}
	t.Setenv(EnvBot, "@OtherBot")
	if Username() != DefaultUsername {
		t.Fatalf("env replaced the published name: %s", Username())
	}
	if !UseDBUsername("TellomiBot") {
		t.Fatal("mismatched env was not reported")
	}
	if Username() != DefaultUsername {
		t.Fatalf("published %s", Username())
	}
	t.Setenv(EnvBot, "TellomiBot")
	if UseDBUsername("TellomiBot") {
		t.Fatal("matching env was reported as a mismatch")
	}
}

func TestAddrAllowed(t *testing.T) {
	if !AddrAllowed("127.0.0.1:8818") || !AddrAllowed("[::1]:8818") || !AddrAllowed("localhost:8818") {
		t.Fatal("loopback addresses were refused")
	}
	if AddrAllowed("10.0.0.4:8818") || AddrAllowed("192.168.1.1:8818") || AddrAllowed("172.16.0.1:8818") || AddrAllowed("0.0.0.0:8818") || AddrAllowed("8.8.8.8:8818") || AddrAllowed("8818") {
		t.Fatal("non-loopback bind was allowed")
	}
}

type fakeList struct {
	rows []Row
	q    Query
}

func (f *fakeList) List(_ context.Context, q Query) ([]Row, error) {
	f.q = q
	var out []Row
	for _, row := range f.rows {
		if q.Sender != 0 && row.SenderUserID != q.Sender {
			continue
		}
		if !q.From.IsZero() && row.CreatedAt.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && row.CreatedAt.After(q.To) {
			continue
		}
		if q.Device != "" && !strings.Contains(strings.ToLower(row.Device+row.Filename+row.Message), strings.ToLower(q.Device)) {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func TestQueryFiltersAndAuth(t *testing.T) {
	when := time.Date(2026, 11, 10, 1, 0, 0, 0, time.UTC)
	fake := &fakeList{rows: []Row{
		{ID: 1, CreatedAt: when, SenderUserID: 9, MsgID: 3, Device: "iOS", Filename: "a.zip", StorageKey: "crash-reports/2026/11/10/9/3_a.zip", DownloadURL: "https://example/a"},
		{ID: 2, CreatedAt: when.Add(time.Hour), SenderUserID: 8, MsgID: 4, Device: "Android", Filename: "b.zip"},
	}}
	h := Handler{Service: fake, Token: "test-token"}
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/crash-reports?sender=9&from=1762736400&device=ios", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	// 2026-11-10 01:00 UTC is 1762736400? Compute from the row instead of a guessed unix.
	from := when.Add(-time.Minute).Unix()
	to := when.Add(time.Minute).Unix()
	req = httptest.NewRequest(http.MethodGet, "/internal/v1/crash-reports?sender=9&device=ios&from="+strconv.FormatInt(from, 10)+"&to="+strconv.FormatInt(to, 10), nil)
	req.Header.Set("X-Admin-Token", "test-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Reports []Row `json:"reports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Reports) != 1 || body.Reports[0].SenderUserID != 9 || body.Reports[0].DownloadURL == "" {
		t.Fatalf("%+v", body.Reports)
	}
	req = httptest.NewRequest(http.MethodGet, "/internal/v1/crash-reports?sender=9&from="+strconv.FormatInt(when.Add(2*time.Hour).Unix(), 10), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Reports) != 0 {
		t.Fatalf("future window %+v", body.Reports)
	}
}

func TestMigrationDoesNotTouchOtherUsers(t *testing.T) {
	raw, err := os.ReadFile("../../teamgramd/deploy/sql/migrate-20261125-crash-report-bot.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS crash_reports",
		"INSERT IGNORE INTO users",
		"777006",
		"TellomiBot",
		"crash bot username taken:",
		"crash bot user id taken:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("migration missing %s", want)
		}
	}
	if strings.Contains(strings.ToUpper(text), "DELETE FROM") || strings.Contains(text, "FLUSHALL") {
		t.Fatal("migration deletes rows")
	}
	if !strings.Contains(text, "WHERE id = 777006 AND user_type = 3 AND is_bot = 1 AND username = ''") {
		t.Fatal("user update is not limited to the empty crash-bot row")
	}
}

type countObjects struct {
	copies int
	puts   int
}

func (c *countObjects) Copy(context.Context, string, string, string) error {
	c.copies++
	return nil
}

func (c *countObjects) Put(context.Context, string, string, []byte) error {
	c.puts++
	return nil
}

func (c *countObjects) Presign(context.Context, string, time.Duration) (string, error) {
	return "https://example/signed", nil
}

func TestOverCapSkipsCopy(t *testing.T) {
	t.Setenv(EnvMaxBytes, "100")
	if MaxBytes() != 100 {
		t.Fatalf("cap %d", MaxBytes())
	}
	t.Setenv(EnvMaxBytes, "")
	if MaxBytes() != DefaultMaxBytes {
		t.Fatalf("default cap %d", MaxBytes())
	}
	t.Setenv(EnvMaxBytes, "100")
	objs := &countObjects{}
	var saved Row
	s := &Service{
		objects: objs,
		docs: func(context.Context, int64) (docSource, error) {
			return docSource{Class: "documents", Path: "1.dat", Size: 101, Name: "big.zip", Mime: "application/zip"}, nil
		},
		saveRow: func(_ context.Context, row Row) error {
			saved = row
			return nil
		},
	}
	if err := s.capture(context.Background(), Incoming{SenderID: 9, MsgID: 3, DocumentID: 5, Filename: "big.zip"}); err != nil {
		t.Fatal(err)
	}
	if objs.copies != 0 || objs.puts != 0 || saved.Status != StatusSkipped || saved.StorageKey != "" || saved.FileSize != 101 {
		t.Fatalf("copies %d puts %d row %+v", objs.copies, objs.puts, saved)
	}
	s.docs = func(context.Context, int64) (docSource, error) {
		return docSource{Class: "documents", Path: "1.dat", Size: 40, Name: "ok.zip", Mime: "application/zip"}, nil
	}
	if err := s.capture(context.Background(), Incoming{SenderID: 9, MsgID: 4, DocumentID: 6, Filename: "ok.zip"}); err != nil {
		t.Fatal(err)
	}
	if objs.copies != 1 || saved.Status != StatusStored || saved.StorageKey == "" || saved.FileSize != 40 {
		t.Fatalf("copies %d row %+v", objs.copies, saved)
	}
}

func TestOurBotPredicate(t *testing.T) {
	if !ourBot(3, 1, "TellomiBot") || !ourBot(3, 1, "") {
		t.Fatal("own bot rejected")
	}
	if ourBot(2, 0, "TellomiBot") || ourBot(3, 1, "someone") {
		t.Fatal("other account accepted")
	}
}
