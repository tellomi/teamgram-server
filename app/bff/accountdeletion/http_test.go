package accountdeletion

import (
	"context"
	"encoding/json"
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
	deps.AllowSend = func(context.Context, string, string) error {
		return &verify.LimitError{Retry: 60}
	}
	h := New(deps).Handler()
	code, body := post(t, h, "/v1/verification/session", `{"phone":"+8613800138000"}`, origin)
	if code != http.StatusTooManyRequests || body["error"] != "rate_limited" {
		t.Fatalf("%d %#v", code, body)
	}
}
