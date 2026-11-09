package enforcement

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCredentialServiceKey(t *testing.T) {
	s, _ := testSvc(true)
	SetCurrent(s)
	t.Cleanup(func() { SetCurrent(nil) })
	now := s.clock()
	key := testServiceKey()
	h := CredentialHandler{Now: func() time.Time { return now }, Key: key, Verify: subjectMap{"user-3": 3}}
	body := `{"credential":"user-3","write":false,"user_id":42}`

	okReq := httptest.NewRequest(http.MethodPost, "/internal/v1/enforcement/credentials/check", strings.NewReader(body))
	authorizeService(okReq, key)
	okRec := httptest.NewRecorder()
	h.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK || !strings.Contains(okRec.Body.String(), `"allow":true`) || !strings.Contains(okRec.Body.String(), `"status":"clear"`) {
		t.Fatalf("correct key %d %s", okRec.Code, okRec.Body.String())
	}

	missing := postCredential(t, h, body, "")
	wrongKey := bytesKey(0x11)
	wrong := postCredential(t, h, body, "Bearer "+base64.StdEncoding.EncodeToString(wrongKey))
	plain := postCredential(t, h, body, "Bearer not-base64")
	unset := CredentialHandler{Now: h.Now, Verify: h.Verify}
	unconfigured := postCredential(t, unset, body, "Bearer "+base64.StdEncoding.EncodeToString(key))
	for _, rec := range []*httptest.ResponseRecorder{missing, wrong, plain, unconfigured} {
		if rec.Code != http.StatusUnauthorized || rec.Body.String() != missing.Body.String() {
			t.Fatalf("auth failure diverged: %d %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"allow":true`) || strings.Contains(strings.ToLower(rec.Body.String()), "missing") || strings.Contains(strings.ToLower(rec.Body.String()), "wrong") || strings.Contains(strings.ToLower(rec.Body.String()), "config") {
			t.Fatal(rec.Body.String())
		}
	}

	t.Setenv(EnvServiceKey, "")
	if _, err := LoadServiceKey(); err == nil {
		t.Fatal("empty service key was accepted")
	}
	t.Setenv(EnvServiceKey, base64.StdEncoding.EncodeToString([]byte("short")))
	if _, err := LoadServiceKey(); err == nil {
		t.Fatal("short service key was accepted")
	}
	t.Setenv(EnvServiceKey, base64.StdEncoding.EncodeToString(key))
	loaded, err := LoadServiceKey()
	if err != nil || len(loaded) != len(key) {
		t.Fatal(err)
	}
	loadedHandler := CredentialHandler{Now: h.Now, Key: loaded, Verify: h.Verify}
	loadedRec := postCredential(t, loadedHandler, body, "Bearer "+base64.StdEncoding.EncodeToString(loaded))
	if loadedRec.Code != http.StatusOK || !strings.Contains(loadedRec.Body.String(), `"allow":true`) {
		t.Fatalf("loaded key %d %s", loadedRec.Code, loadedRec.Body.String())
	}
}

func postCredential(t *testing.T, h CredentialHandler, body, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/enforcement/credentials/check", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func bytesKey(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}
