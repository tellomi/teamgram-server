package enforcement

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/pkg/discoverauth"
)

// TestLoopbackContract is an HTTP call against 127.0.0.1, using the same
// discover JWT bytes content-engine 1347091 rechecks. content-engine itself
// was not readable from this environment (GitHub 404), so this is not that
// process. It is the server handler on a real loopback socket.
func TestLoopbackContract(t *testing.T) {
	s, _ := testSvc(true)
	SetCurrent(s)
	t.Cleanup(func() { SetCurrent(nil) })
	now := s.clock().UTC().Truncate(time.Second)
	end := now.Add(time.Hour)
	key := []byte("loopback-pseudonym-key")

	perm, err := s.Create(Actor{ID: "s1", Role: RoleSenior}, Sanction{
		UserID: 42, Kind: KindPermanent, Permanent: true, ReasonCategory: "类别", PolicyClause: "条款", RuleVersion: "rule-1", Start: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Approve(Actor{ID: "s2", Role: RoleSenior}, perm.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(Actor{ID: "a", Role: RoleAgent}, Sanction{
		UserID: 4, Kind: KindScoped, Scopes: []string{ScopeDiscover},
		ReasonCategory: "类别", PolicyClause: "条款", RuleVersion: "rule-1",
		Start: now, End: &end, IdempotencyKey: "loop-disc",
	}); err != nil {
		t.Fatal(err)
	}
	s.Lock(8)
	for _, id := range []int64{42, 4, 8, 3} {
		if err = s.RecordSubject(key, id); err != nil {
			t.Fatal(err)
		}
	}
	login := s.Stage(42, "", "", true, now)
	if login.Grant == nil {
		t.Fatal("no disposition grant")
	}
	if err = s.StashProof("hash-42", *login.Grant); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	svcKey := testServiceKey()
	cred := CredentialHandler{Now: func() time.Time { return now }, Key: svcKey, Verify: subjectFunc(func(token string, at time.Time) (int64, error) {
		return FeedSubject(key, s.UserForSubject, token, at)
	})}
	mux.Handle("/internal/v1/enforcement/credentials/check", cred)
	mux.Handle("/internal/v1/enforcement/disposition/", DispositionHandler{Now: func() time.Time { return now }})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })
	if !strings.HasPrefix(ln.Addr().String(), "127.0.0.1:") {
		t.Fatal(ln.Addr())
	}
	base := "http://" + ln.Addr().String()

	token := func(user int64) string {
		t.Helper()
		tok, err := discoverauth.Sign(key, discoverauth.Pseudonym(key, user), discoverauth.RegionGlobal, now.Add(time.Hour).Unix())
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	check := func(body string, header string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, base+"/internal/v1/enforcement/credentials/check", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		authorizeService(req, svcKey)
		if header != "" {
			req.Header.Set("X-User-Id", header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	code, raw := check(`{"credential":"`+token(3)+`","write":false,"user_id":42}`, "42")
	if code != http.StatusOK || !strings.Contains(raw, `"allow":true`) || !strings.Contains(raw, `"status":"clear"`) {
		t.Fatalf("clear browse %d %s", code, raw)
	}
	code, raw = check(`{"credential":"`+token(4)+`","write":false,"user_id":42}`, "42")
	if code != http.StatusOK || !strings.Contains(raw, `"allow":true`) {
		t.Fatalf("discover browse %d %s", code, raw)
	}
	code, raw = check(`{"credential":"`+token(4)+`","write":true}`, "")
	if code != http.StatusOK || !strings.Contains(raw, `"allow":false`) || !strings.Contains(raw, "ACCOUNT_SCOPED_RESTRICTION") {
		t.Fatalf("discover post %d %s", code, raw)
	}
	code, raw = check(`{"credential":"`+token(42)+`","write":false}`, "7")
	if code != http.StatusOK || !strings.Contains(raw, `"allow":false`) || !strings.Contains(raw, "ACCOUNT_PERMANENT_DISABLED") {
		t.Fatalf("permanent read %d %s", code, raw)
	}
	code, raw = check(`{"credential":"`+token(42)+`","write":true}`, "")
	if !strings.Contains(raw, `"allow":false`) {
		t.Fatalf("permanent write %d %s", code, raw)
	}
	code, raw = check(`{"credential":"`+token(8)+`","write":false}`, "")
	if code != http.StatusOK || !strings.Contains(raw, `"allow":true`) || !strings.Contains(raw, "security_lock") {
		t.Fatalf("lock browse %d %s", code, raw)
	}
	code, raw = check(`{"credential":"`+token(8)+`","write":true}`, "")
	if !strings.Contains(raw, `"allow":false`) || !strings.Contains(raw, "ACCOUNT_SECURITY_LOCK") {
		t.Fatalf("lock post %d %s", code, raw)
	}
	unbound, err := discoverauth.Sign(key, discoverauth.Pseudonym(key, 99), discoverauth.RegionGlobal, now.Add(time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	code, raw = check(`{"credential":"`+unbound+`","write":true,"user_id":42}`, "42")
	if code != http.StatusForbidden || !strings.Contains(raw, "enforcement_unavailable") || strings.Contains(raw, `"allow":true`) {
		t.Fatalf("unbound %d %s", code, raw)
	}
	code, raw = check(`{"credential":"42","write":false,"user_id":42}`, "42")
	if code != http.StatusServiceUnavailable || strings.Contains(raw, `"allow":true`) || !strings.Contains(raw, `"status":"unavailable"`) {
		t.Fatalf("numeric sub %d %s", code, raw)
	}

	post := func(path, body, auth string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	code, raw = post("/internal/v1/enforcement/disposition/notices", `{"user_id":7}`, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("appeal notices without grant %d %s", code, raw)
	}
	code, raw = post("/internal/v1/enforcement/disposition/appeals", `{"sanction_id":"`+perm.ID+`","text":"x","user_id":7}`, "Disposition nope")
	if code != http.StatusUnauthorized {
		t.Fatalf("bad grant %d %s", code, raw)
	}
	code, raw = post("/internal/v1/enforcement/disposition/claim", `{"proof":"hash-42","user_id":7}`, "")
	if code != http.StatusOK || !strings.Contains(raw, login.Grant.Token) {
		t.Fatalf("claim %d %s", code, raw)
	}
	code, raw = post("/internal/v1/enforcement/disposition/notices", `{"user_id":7}`, "Disposition "+login.Grant.Token)
	if code != http.StatusOK || !strings.Contains(raw, perm.ID) {
		t.Fatalf("notices %d %s", code, raw)
	}
	code, raw = post("/internal/v1/enforcement/disposition/appeals", `{"sanction_id":"`+perm.ID+`","text":"review","user_id":7}`, "Disposition "+login.Grant.Token)
	if code != http.StatusOK || !strings.Contains(raw, "pending_review") {
		t.Fatalf("appeal %d %s", code, raw)
	}
	code, raw = post("/internal/v1/enforcement/disposition/send", `{}`, "Disposition "+login.Grant.Token)
	if code != http.StatusNotFound {
		t.Fatalf("send route %d %s", code, raw)
	}

	SetCurrent(nil)
	code, raw = check(`{"credential":"`+token(3)+`","write":true}`, "")
	if code != http.StatusServiceUnavailable || strings.Contains(raw, `"allow":true`) || !strings.Contains(raw, `"status":"disabled"`) {
		t.Fatalf("disabled %d %s", code, raw)
	}
}
