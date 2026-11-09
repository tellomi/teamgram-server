package enforcement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var caseIDPattern = regexp.MustCompile(`C[0-9a-f]{16}`)

func TestParseAdminsIsExtensible(t *testing.T) {
	got := ParseAdmins(" @Erel, erel, ada , ,")
	if strings.Join(got, ",") != "erel,ada" {
		t.Fatal(got)
	}
}

func TestParseAdminText(t *testing.T) {
	for _, text := range []string{"是", " 是 ", "\t是\n", "\u3000是\u3000"} {
		answer, id, ok := parseAdminText(text)
		if !ok || answer != "yes" || id != "" {
			t.Fatalf("%q -> %s %s %v", text, answer, id, ok)
		}
	}
	for _, text := range []string{"否", " 否 ", "\n否\t", "\u3000否\u3000"} {
		answer, id, ok := parseAdminText(text)
		if !ok || answer != "no" || id != "" {
			t.Fatalf("%q -> %s %s %v", text, answer, id, ok)
		}
	}
	rejected := []string{
		"", " ", "是的", " 是的 ", "是 的", "yes", "Y", "y", "YES", "Yes",
		"no", "N", "n", "NO", "否的", "是否", "是是", "是。", "是!",
		"好的", "是 C0123456789abcdef", "否 C0123456789abcdef",
		"  是   C0123456789abcdef  ", "是 C0123456789abcdef C0123456789abcdee",
		"yes C0123456789abcdef", "是的 C0123456789abcdef", "no C0123456789abcdef",
		"y C0123456789abcdef", "C0123456789abcdef",
	}
	for _, text := range rejected {
		if answer, id, ok := parseAdminText(text); ok {
			t.Fatalf("%q was accepted as %s %s", text, answer, id)
		}
	}
}

type memDir struct {
	name map[int64]string
	id   map[string]int64
}

func (d memDir) Username(_ context.Context, userID int64) (string, error) {
	return d.name[userID], nil
}

func (d memDir) UserID(_ context.Context, username string) (int64, error) {
	return d.id[normalizeName(username)], nil
}

type sentBox struct {
	mu   sync.Mutex
	next int32
	rows []sentRow
}

type sentRow struct {
	to   int64
	text string
	id   int32
}

func (b *sentBox) Send(_ context.Context, to int64, text string) (int32, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	b.rows = append(b.rows, sentRow{to: to, text: text, id: b.next})
	return b.next, nil
}

func (b *sentBox) last() sentRow {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rows[len(b.rows)-1]
}

type scriptAI struct {
	fn func(system, prompt string) (string, error)
}

func (s scriptAI) CompletePrompt(_ context.Context, _, system, prompt string) (string, int, error) {
	text, err := s.fn(system, prompt)
	return text, 3, err
}

func testConfirm(t *testing.T, ai scriptAI, admins ...string) (*Confirmer, *sentBox, *Service) {
	t.Helper()
	svc := New(Config{CreateEnabled: true, Now: func() time.Time {
		return time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	}})
	box := &adminBox{}
	svc.SetSender(box)
	out := &sentBox{}
	dir := memDir{name: map[int64]string{8: "erel", 9: "ada", 4: "alice"}, id: map[string]int64{"erel": 8, "ada": 9}}
	c := &Confirmer{Svc: svc, AI: ai, Model: "fake", Admins: admins, Threshold: 0.8, Dir: dir, Send: out.Send, Now: svc.now}
	UseConfirmer(c)
	t.Cleanup(func() { UseConfirmer(nil); SetCurrent(nil) })
	SetCurrent(svc)
	return c, out, svc
}

type adminBox struct {
	mu     sync.Mutex
	bodies []string
}

func (b *adminBox) SendFromAdmin(_ int64, _, body string) error {
	b.mu.Lock()
	b.bodies = append(b.bodies, body)
	b.mu.Unlock()
	return nil
}

func substantiated(system, prompt string) (string, error) {
	if !strings.Contains(system, accountMarker) {
		return "", io.EOF
	}
	return `{"substantiated":true,"confidence":0.91,"reason":"辱骂重复出现","summary":"私聊里有辱骂，资料也在引流"}`, nil
}

func TestAccountYesBansAndNoDoesNot(t *testing.T) {
	c, out, svc := testConfirm(t, scriptAI{fn: substantiated}, "erel", "ada")
	c.OnAccount(context.Background(), AccountInput{ReportID: 7, SubjectUserID: 42, Label: "42 @bad 坏", Reason: "spam", Messages: []string{"去你的"}, Profile: "42 @bad", Prior: []string{"spam 上次"}})
	msg := out.rows[0]
	if msg.to != 8 || !strings.Contains(msg.text, "42 @bad") || !strings.Contains(msg.text, "辱骂重复出现") || !strings.Contains(msg.text, "案件号：C") {
		t.Fatalf("admin message: %+v", msg)
	}
	if !strings.Contains(out.rows[1].text, "案件号：") || out.rows[1].to != 9 {
		t.Fatal("second admin was not notified")
	}
	caseID := caseIDPattern.FindString(msg.text)
	if ObserveAdmin(context.Background(), 4, "是", msg.id) {
		t.Fatal("non-admin was treated as an admin")
	}
	if d := svc.Decide(42, "send_message", false, svc.now()); !d.Allowed {
		t.Fatal("non-admin reply banned the account")
	}
	reply := c.answer(context.Background(), 8, "erel", "看看", msg.id)
	if reply != helpText {
		t.Fatal(reply)
	}
	if d := svc.Decide(42, "send_message", false, svc.now()); !d.Allowed {
		t.Fatal("help text banned the account")
	}

	c.OnAccount(context.Background(), AccountInput{ReportID: 8, SubjectUserID: 43, Label: "43 @other", Reason: "spam"})
	second := out.last()
	secondID := caseIDPattern.FindString(second.text)
	if caseID == secondID {
		t.Fatal("cases collided")
	}
	ambiguous := c.answer(context.Background(), 8, "erel", "是", 0)
	if !strings.Contains(ambiguous, "回复对应") {
		t.Fatal(ambiguous)
	}
	if named := c.answer(context.Background(), 8, "erel", "是 "+secondID, 0); named != helpText {
		t.Fatal(named)
	}
	for _, loose := range []string{"是的", "yes", "y", "Y", "no", "n"} {
		reply := c.answer(context.Background(), 9, "ada", loose, second.id)
		if reply != helpText {
			t.Fatalf("%q -> %s", loose, reply)
		}
	}
	if d := svc.Decide(43, "send_message", false, svc.now()); !d.Allowed {
		t.Fatal("a loose reply banned the account")
	}
	if d := svc.Decide(42, "send_message", false, svc.now()); !d.Allowed {
		t.Fatal("a loose reply banned the other account")
	}
	yes := c.answer(context.Background(), 9, "ada", " 是 ", second.id)
	if !strings.Contains(yes, "已封禁") || !strings.Contains(yes, secondID) {
		t.Fatal(yes)
	}
	if d := svc.Decide(43, "send_message", false, svc.now()); d.Allowed || d.Code != "ACCOUNT_PERMANENT_DISABLED" {
		t.Fatalf("banned user can still send: %+v", d)
	}
	if d := svc.Decide(42, "send_message", false, svc.now()); !d.Allowed {
		t.Fatal("the other reported user was banned")
	}
	notice := svc.notices
	var body string
	for _, n := range notice {
		if n.UserID == 43 && n.From == 777000 {
			body = n.Body
		}
	}
	if !strings.Contains(body, "处置编号") {
		t.Fatalf("777000 notice: %q", body)
	}
	var sawYes, sawAI bool
	for _, a := range svc.audits {
		if a.Action == "admin_yes" && a.Actor == "ada" && strings.Contains(a.Detail, "answer=是") && a.CaseID == secondID {
			sawYes = true
		}
		if a.Action == "ai_account" && a.Actor == actorAI && strings.Contains(a.Detail, "辱骂") {
			sawAI = true
		}
	}
	if !sawYes || !sawAI {
		t.Fatalf("audit: %+v", svc.audits)
	}
	if namedNo := c.answer(context.Background(), 8, "erel", "否 "+caseID, 0); namedNo != helpText {
		t.Fatal(namedNo)
	}
	still, ok, err := svc.book().get(caseID)
	if err != nil || !ok || still.Status != reviewPending {
		t.Fatalf("case id reply changed the case: %+v %v", still, err)
	}
	no := c.answer(context.Background(), 8, "erel", " 否 ", msg.id)
	if !strings.Contains(no, "没有封禁") {
		t.Fatal(no)
	}
	if d := svc.Decide(42, "send_message", false, svc.now()); !d.Allowed {
		t.Fatal("否 banned the account")
	}
	closed, ok, err := svc.book().get(caseID)
	if err != nil || !ok || closed.Status != reviewClosed || closed.AdminAnswer != "否" {
		t.Fatalf("%+v %v", closed, err)
	}
}

func TestContentAutonomyAndAdmin(t *testing.T) {
	var mode string
	c, out, _ := testConfirm(t, scriptAI{fn: func(system, prompt string) (string, error) {
		if !strings.Contains(system, contentMarker) {
			t.Fatal("content marker missing")
		}
		switch mode {
		case "low":
			return `{"action":"remove","confidence":0.2,"reason":"拿不准","summary":"像广告"}`, nil
		case "keep":
			return `{"action":"keep","confidence":0.95,"reason":"正常","summary":"普通发言"}`, nil
		default:
			return `{"action":"remove","confidence":0.96,"reason":"色情","summary":"正文违规"}`, nil
		}
	}}, "erel")
	high, err := c.ReviewContent(context.Background(), ContentInput{PostID: "p1", AuthorUserID: 5, Text: "违规"})
	if err != nil || high.Decision != "remove" || len(out.rows) != 0 {
		t.Fatalf("%+v %v %d", high, err, len(out.rows))
	}
	mode = "keep"
	kept, err := c.ReviewContent(context.Background(), ContentInput{PostID: "p2", Text: "你好"})
	if err != nil || kept.Decision != "keep" {
		t.Fatal(kept, err)
	}
	mode = "low"
	before := len(out.rows)
	low, err := c.ReviewContent(context.Background(), ContentInput{PostID: "p3", CommentID: "c9", AuthorUserID: 5, Text: "也许"})
	if err != nil || low.Decision != "pending_admin" || len(out.rows) != before+1 {
		t.Fatalf("%+v %v", low, err)
	}
	if !strings.Contains(out.last().text, low.CaseID) {
		t.Fatal(out.last().text)
	}
	if ack := c.answer(context.Background(), 8, "erel", "yes "+low.CaseID, 0); ack != helpText {
		t.Fatal(ack)
	}
	pending, ok, err := c.ContentStatus(low.CaseID)
	if err != nil || !ok || pending.Decision != "pending_admin" {
		t.Fatalf("%+v %v", pending, err)
	}
	ack := c.answer(context.Background(), 8, "erel", " 是 ", 0)
	if !strings.Contains(ack, "下架") {
		t.Fatal(ack)
	}
	got, ok, err := c.ContentStatus(low.CaseID)
	if err != nil || !ok || got.Decision != "remove" || got.Admin != "erel" || got.Answer != "是" {
		t.Fatalf("%+v %v", got, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/enforcement/content/decision", strings.NewReader(`{"case_id":"`+low.CaseID+`"}`))
	key := confirmKey()
	req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(key))
	rec := httptest.NewRecorder()
	ContentHandler{Key: key}.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"decision":"remove"`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
	denied := httptest.NewRequest(http.MethodPost, "/internal/v1/enforcement/content/decision", strings.NewReader(`{"case_id":"`+low.CaseID+`"}`))
	rec = httptest.NewRecorder()
	ContentHandler{Key: key}.ServeHTTP(rec, denied)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal(rec.Code)
	}
}

func TestServeNetworkRefusesPublicIP(t *testing.T) {
	if err := ServeNetwork("1.2.3.4:8820"); err == nil {
		t.Fatal("public address was accepted")
	}
	if err := ServeNetwork("8820"); err == nil {
		t.Fatal("missing host was accepted")
	}
}

func TestStoreSwitch(t *testing.T) {
	SetCurrent(nil)
	t.Cleanup(func() { SetCurrent(nil) })
	t.Setenv(EnvStore, "")
	t.Setenv(EnvMySQL, "root:@tcp(127.0.0.1:1)/tellomi_enforcement?parseTime=true")
	if InstallFromEnv() != nil {
		t.Fatal("empty store installed")
	}
	t.Setenv(EnvStore, "memory")
	if InstallFromEnv() != nil {
		t.Fatal("memory store installed")
	}
	t.Setenv(EnvStore, "mysql")
	t.Setenv(EnvMySQL, "")
	if InstallFromEnv() != nil {
		t.Fatal("mysql switch without a dsn installed")
	}
}

func confirmKey() []byte {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	return raw
}

func TestContentReviewJSONRoundTrip(t *testing.T) {
	body, _ := json.Marshal(ContentDecision{CaseID: "C0123456789abcdef", Decision: "pending_admin"})
	if !strings.Contains(string(body), "pending_admin") {
		t.Fatal(string(body))
	}
}
