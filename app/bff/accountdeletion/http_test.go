package accountdeletion

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/verify"
)

const origin = "https://chat.tellomi.cn"

func testDeps(acc Account, hasPass bool) (Deps, *[]int64) {
	var purged []int64
	return Deps{
		Lookup: func(context.Context, string) (Account, bool, error) {
			if acc.UserID == 0 {
				return Account{}, false, nil
			}
			return acc, true, nil
		},
		Password: func(context.Context, int64) (bool, int64, string, error) {
			if hasPass {
				return true, 1, "pet", nil
			}
			return false, 0, "", nil
		},
		CheckPassword: func(_ context.Context, _ int64, plain string) error {
			if plain != "secret" {
				return errPass
			}
			return nil
		},
		Purge: func(_ context.Context, userID int64, _ string) error {
			purged = append(purged, userID)
			return nil
		},
		FixedCode: "12345",
		Origins:   []string{origin},
	}, &purged
}

type errPassT struct{}

func (errPassT) Error() string { return "bad" }

var errPass = errPassT{}

func post(t *testing.T, h http.Handler, path, body, orig string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if orig != "" {
		req.Header.Set("Origin", orig)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestOriginRequired(t *testing.T) {
	deps, _ := testDeps(Account{UserID: 7, Phone: "8613800138000"}, false)
	h := New(deps).Handler()
	code, body := post(t, h, "/v1/verification/session", `{"phone":"+8613800138000"}`, "")
	if code != http.StatusForbidden || body["error"] != "origin" {
		t.Fatalf("%d %#v", code, body)
	}
	code, _ = post(t, h, "/v1/verification/session", `{"phone":"+8613800138000"}`, "https://evil.example")
	if code != http.StatusForbidden {
		t.Fatalf("evil %d", code)
	}
}

func TestMissingAccountMatchesRealResponse(t *testing.T) {
	real, _ := testDeps(Account{UserID: 7, Phone: "8613800138000"}, false)
	missing, _ := testDeps(Account{}, false)
	hr, hm := New(real).Handler(), New(missing).Handler()
	cr, br := post(t, hr, "/v1/verification/session", `{"phone":"+8613800138000"}`, origin)
	cm, bm := post(t, hm, "/v1/verification/session", `{"phone":"+8613800138000"}`, origin)
	if cr != http.StatusOK || cm != http.StatusOK {
		t.Fatalf("real %d missing %d", cr, cm)
	}
	if br["expires_in"] != bm["expires_in"] || br["code_length"] != bm["code_length"] {
		t.Fatalf("shape %#v %#v", br, bm)
	}
	if br["session_id"] == "" || bm["session_id"] == "" || br["session_id"] == bm["session_id"] {
		t.Fatalf("sessions %#v %#v", br, bm)
	}
}

func TestDeleteRequiresCodePasswordAndConfirm(t *testing.T) {
	deps, purged := testDeps(Account{UserID: 7, Phone: "8613800138000"}, true)
	h := New(deps).Handler()
	_, sess := post(t, h, "/v1/verification/session", `{"phone":"8613800138000"}`, origin)
	id, _ := sess["session_id"].(string)

	code, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"00000","confirm":"DELETE"}`, origin)
	if code != http.StatusBadRequest || body["error"] != "code_invalid" {
		t.Fatalf("bad code %d %#v", code, body)
	}
	code, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"12345","confirm":"DELETE"}`, origin)
	if code != http.StatusConflict || body["error"] != "password_required" || body["hint"] != "pet" {
		t.Fatalf("password %d %#v", code, body)
	}
	code, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"nope","confirm":"DELETE"}`, origin)
	if code != http.StatusBadRequest || body["error"] != "password_invalid" {
		t.Fatalf("wrong password %d %#v", code, body)
	}
	code, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"secret","confirm":"yes"}`, origin)
	if code != http.StatusBadRequest || body["error"] != "confirm_required" {
		t.Fatalf("confirm %d %#v", code, body)
	}
	code, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"secret","confirm":"DELETE"}`, origin)
	if code != http.StatusOK || body["deleted"] != true {
		t.Fatalf("delete %d %#v", code, body)
	}
	if len(*purged) != 1 || (*purged)[0] != 7 {
		t.Fatalf("purged %#v", *purged)
	}
	code, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"secret","confirm":"DELETE"}`, origin)
	if code != http.StatusBadRequest {
		t.Fatalf("reuse %d %#v", code, body)
	}
}

func TestUnknownPhoneCodeIsRejected(t *testing.T) {
	deps, purged := testDeps(Account{}, false)
	h := New(deps).Handler()
	_, sess := post(t, h, "/v1/verification/session", `{"phone":"+8613800138000"}`, origin)
	id, _ := sess["session_id"].(string)
	code, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"12345","confirm":"DELETE"}`, origin)
	if code != http.StatusBadRequest || body["error"] != "code_invalid" {
		t.Fatalf("%d %#v", code, body)
	}
	if len(*purged) != 0 {
		t.Fatal("purged missing account")
	}
}

func TestPageDoesNotAdvertiseHiddenLogin(t *testing.T) {
	deps, _ := testDeps(Account{}, false)
	req := httptest.NewRequest(http.MethodGet, "/delete-account/", nil)
	rec := httptest.NewRecorder()
	New(deps).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, bad := range []string{"加密", "端到端", "微信", "微博", "Apple", "Google"} {
		if strings.Contains(text, bad) {
			t.Fatalf("page contains %s", bad)
		}
	}
}

func TestRedactDigits(t *testing.T) {
	if RedactDigits("send failed code 123456") != "send failed code ****" {
		t.Fatal(RedactDigits("send failed code 123456"))
	}
}

func TestRateLimit(t *testing.T) {
	deps, _ := testDeps(Account{UserID: 7}, false)
	var kind, target string
	deps.AllowSend = func(_ context.Context, k, tgt, _ string) error {
		kind, target = k, tgt
		return &verify.LimitError{Retry: 60}
	}
	h := New(deps).Handler()
	code, body := post(t, h, "/v1/verification/session", `{"phone":"+8613800138000"}`, origin)
	if code != http.StatusTooManyRequests || body["error"] != "rate_limited" {
		t.Fatalf("%d %#v", code, body)
	}
	if kind != "sms" || target != "8613800138000" {
		t.Fatalf("quota %s %s", kind, target)
	}
}

func TestConfirmWordExact(t *testing.T) {
	for _, word := range []string{"DELETE", "删除"} {
		if !confirmAccepted(word) {
			t.Fatalf("accepted word rejected: %q", word)
		}
	}
	for _, word := range []string{"delete", "Delete", "DELETE ", " DELETE", "删除 ", " 删除", "删除账号", "刪除", ""} {
		if confirmAccepted(word) {
			t.Fatalf("rejected word accepted: %q", word)
		}
	}

	deps, purged := testDeps(Account{UserID: 7, Phone: "8613800138000"}, false)
	h := New(deps).Handler()
	_, sess := post(t, h, "/v1/verification/session", `{"phone":"+8613800138000"}`, origin)
	id, _ := sess["session_id"].(string)
	for _, bad := range []string{"delete", "Delete", "DELETE ", "删除 ", "删除账号"} {
		code, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"12345","confirm":"`+bad+`"}`, origin)
		if code != http.StatusBadRequest || body["error"] != "confirm_required" {
			t.Fatalf("%q %d %#v", bad, code, body)
		}
	}
	code, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"12345","confirm":"删除"}`, origin)
	if code != http.StatusOK || body["deleted"] != true {
		t.Fatalf("chinese confirm %d %#v", code, body)
	}
	if len(*purged) != 1 {
		t.Fatalf("purged %#v", *purged)
	}
}

func TestEmailModePasswordAndConfirm(t *testing.T) {
	deps, purged := testDeps(Account{UserID: 11}, true)
	deps.FixedCode = "12345"
	deps.FixedEmailCode = "654321"
	deps.Lookup = func(_ context.Context, key string) (Account, bool, error) {
		if key != "user@tellomi.app" {
			t.Fatalf("lookup %q", key)
		}
		return Account{UserID: 11}, true, nil
	}
	smsSends := 0
	deps.SendSMS = func(context.Context, string, string) error {
		smsSends++
		return nil
	}
	emailSends := 0
	deps.SendEmail = func(context.Context, string, string) error {
		emailSends++
		return nil
	}
	var quotaKind, quotaTarget string
	deps.AllowSend = func(_ context.Context, kind, target, _ string) error {
		quotaKind, quotaTarget = kind, target
		return nil
	}
	h := New(deps).Handler()
	status, sess := post(t, h, "/v1/verification/session", `{"email":"User@Tellomi.app"}`, origin)
	if status != http.StatusOK || sess["code_length"] != float64(6) {
		t.Fatalf("session %d %#v", status, sess)
	}
	if quotaKind != "email" || quotaTarget != "user@tellomi.app" {
		t.Fatalf("quota %s %s", quotaKind, quotaTarget)
	}
	if smsSends != 0 || emailSends != 0 {
		t.Fatalf("fixed email code must not send sms=%d email=%d", smsSends, emailSends)
	}
	id, _ := sess["session_id"].(string)

	status, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"12345","confirm":"DELETE"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "code_invalid" {
		t.Fatalf("sms code on email session %d %#v", status, body)
	}
	status, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"654321","confirm":"DELETE"}`, origin)
	if status != http.StatusConflict || body["error"] != "password_required" || body["hint"] != "pet" {
		t.Fatalf("password %d %#v", status, body)
	}
	status, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"nope","confirm":"删除"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "password_invalid" {
		t.Fatalf("wrong password %d %#v", status, body)
	}
	status, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"secret","confirm":"delete"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "confirm_required" {
		t.Fatalf("confirm %d %#v", status, body)
	}
	status, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","password":"secret","confirm":"删除"}`, origin)
	if status != http.StatusOK || body["deleted"] != true {
		t.Fatalf("delete %d %#v", status, body)
	}
	if len(*purged) != 1 || (*purged)[0] != 11 {
		t.Fatalf("purged %#v", *purged)
	}
}

func TestEmailSendQuotaAndHideMiss(t *testing.T) {
	deps, purged := testDeps(Account{}, false)
	deps.FixedCode = "12345"
	deps.FixedEmailCode = ""
	deps.EmailCodeLen = 6
	deps.Lookup = func(_ context.Context, key string) (Account, bool, error) {
		if key == "user@tellomi.app" {
			return Account{UserID: 4}, true, nil
		}
		return Account{}, false, nil
	}
	var sent []string
	deps.SendEmail = func(_ context.Context, to, code string) error {
		sent = append(sent, to+"="+code)
		return nil
	}
	deps.SendSMS = func(context.Context, string, string) error {
		t.Fatal("email mode called SMS")
		return nil
	}
	var quotas []string
	deps.AllowSend = func(_ context.Context, kind, target, _ string) error {
		quotas = append(quotas, kind+" "+target)
		return nil
	}
	h := New(deps).Handler()
	realStatus, real := post(t, h, "/v1/verification/session", `{"email":"user@tellomi.app"}`, origin)
	missStatus, miss := post(t, h, "/v1/verification/session", `{"email":"missing@tellomi.app"}`, origin)
	if realStatus != http.StatusOK || missStatus != http.StatusOK {
		t.Fatalf("real %d miss %d", realStatus, missStatus)
	}
	if real["expires_in"] != miss["expires_in"] || real["code_length"] != miss["code_length"] || real["code_length"] != float64(6) {
		t.Fatalf("shape %#v %#v", real, miss)
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "user@tellomi.app=") {
		t.Fatalf("sent %#v", sent)
	}
	if len(quotas) != 2 || quotas[0] != "email user@tellomi.app" || quotas[1] != "email absent:192.0.2.1" {
		t.Fatalf("quotas %#v", quotas)
	}
	code := strings.TrimPrefix(sent[0], "user@tellomi.app=")
	missID, _ := miss["session_id"].(string)
	status, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+missID+`","code":"`+code+`","confirm":"DELETE"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "code_invalid" {
		t.Fatalf("missing %d %#v", status, body)
	}
	realID, _ := real["session_id"].(string)
	status, body = post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+realID+`","code":"`+code+`","confirm":"DELETE"}`, origin)
	if status != http.StatusOK || body["deleted"] != true {
		t.Fatalf("delete %d %#v", status, body)
	}
	if len(*purged) != 1 || (*purged)[0] != 4 {
		t.Fatalf("purged %#v", *purged)
	}
}

func TestEmailSendFailureMatchesMissing(t *testing.T) {
	deps, purged := testDeps(Account{UserID: 4}, false)
	deps.FixedEmailCode = ""
	var got string
	deps.SendEmail = func(_ context.Context, _, code string) error {
		got = code
		return errors.New("provider code 123456")
	}
	h := New(deps).Handler()
	status, sess := post(t, h, "/v1/verification/session", `{"email":"user@tellomi.app"}`, origin)
	if status != http.StatusOK || sess["session_id"] == "" {
		t.Fatalf("session %d %#v", status, sess)
	}
	id, _ := sess["session_id"].(string)
	status, body := post(t, h, "/v1/tellomi/account-deletion", `{"session_id":"`+id+`","code":"`+got+`","confirm":"DELETE"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "code_invalid" {
		t.Fatalf("failed send %d %#v", status, body)
	}
	if len(*purged) != 0 {
		t.Fatal("purged after failed send")
	}
}

func TestEmailRateLimitDoesNotSend(t *testing.T) {
	deps, _ := testDeps(Account{UserID: 4}, false)
	deps.FixedEmailCode = ""
	sent := 0
	deps.SendEmail = func(context.Context, string, string) error {
		sent++
		return nil
	}
	deps.AllowSend = func(_ context.Context, kind, target, _ string) error {
		if kind != "email" || target != "user@tellomi.app" {
			t.Fatalf("quota %s %s", kind, target)
		}
		return &verify.LimitError{Retry: 30}
	}
	h := New(deps).Handler()
	status, body := post(t, h, "/v1/verification/session", `{"email":"user@tellomi.app"}`, origin)
	if status != http.StatusTooManyRequests || body["error"] != "rate_limited" || body["retry_after"] != float64(30) {
		t.Fatalf("%d %#v", status, body)
	}
	if sent != 0 {
		t.Fatal("sent despite rate limit")
	}
}

func TestEmailInputRejected(t *testing.T) {
	lookups := 0
	deps, _ := testDeps(Account{UserID: 4}, false)
	deps.Lookup = func(context.Context, string) (Account, bool, error) {
		lookups++
		return Account{}, false, nil
	}
	h := New(deps).Handler()
	status, body := post(t, h, "/v1/verification/session", `{"email":"not-an-email"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "email_invalid" {
		t.Fatalf("invalid %d %#v", status, body)
	}
	status, body = post(t, h, "/v1/verification/session", `{"email":"a@b.com","phone":"+8613800138000"}`, origin)
	if status != http.StatusBadRequest || body["error"] != "bad_request" {
		t.Fatalf("mixed %d %#v", status, body)
	}
	if lookups != 0 {
		t.Fatalf("lookup on bad input %d", lookups)
	}
}

func TestPageModesCSPAndConfirmCopy(t *testing.T) {
	deps, _ := testDeps(Account{}, false)
	h := New(deps).Handler()
	for _, path := range []string{"/delete-account/", "/delete-account"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d", path, rec.Code)
		}
		body, _ := io.ReadAll(rec.Body)
		text := string(body)
		for _, bad := range []string{"加密", "端到端", "微信", "微博", "Apple", "Google"} {
			if strings.Contains(text, bad) {
				t.Fatalf("page contains %s", bad)
			}
		}
		if !strings.Contains(text, "手机号") || !strings.Contains(text, "邮箱") {
			t.Fatal("modes missing")
		}
		if !strings.Contains(text, `输入 <span class="word">删除</span> 以确认删除`) {
			t.Fatal("chinese confirm label missing")
		}
		if !strings.Contains(text, `["Type ", "DELETE", " to confirm"]`) || !strings.Contains(text, `["输入 ", "删除", " 以确认删除"]`) {
			t.Fatal("locale confirm copy missing")
		}
		if !strings.Contains(text, "typed !== expected()") {
			t.Fatal("client does not require an exact confirm word")
		}
		if !strings.Contains(text, ".word { color: #c00;") {
			t.Fatal("confirm word is not red")
		}
		style := extractTag(t, text, "style")
		script := extractTag(t, text, "script")
		csp := rec.Header().Get("Content-Security-Policy")
		if csp != contentSecurityPolicy(style, script) || csp != pageCSP {
			t.Fatalf("csp mismatch %s", csp)
		}
		for _, want := range []string{
			"default-src 'none'",
			"base-uri 'none'",
			"object-src 'none'",
			"connect-src 'self'",
			"frame-ancestors 'none'",
			"script-src 'sha256-",
			"style-src 'sha256-",
		} {
			if !strings.Contains(csp, want) {
				t.Fatalf("csp missing %s in %s", want, csp)
			}
		}
		if strings.Contains(csp, "'unsafe-inline'") || strings.Contains(csp, "'unsafe-eval'") {
			t.Fatal(csp)
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatal(rec.Header().Get("X-Frame-Options"))
		}
		sum := sha256.Sum256([]byte(script))
		if !strings.Contains(csp, base64.StdEncoding.EncodeToString(sum[:])) {
			t.Fatal("script hash not in csp")
		}
	}
}

func extractTag(t *testing.T, html, tag string) string {
	t.Helper()
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatalf("no %s", tag)
	}
	i += len(open)
	j := strings.Index(html[i:], close)
	if j < 0 {
		t.Fatalf("no close %s", tag)
	}
	return html[i : i+j]
}
