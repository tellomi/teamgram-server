package enforcement

import (
	"encoding/json"
	"net/http"
	"time"
)

// CredentialHandler is the content-engine recheck. It does not listen unless
// a caller mounts it. The bearer key is required; an empty key refuses every
// request.
type CredentialHandler struct {
	Key string
	Now func() time.Time
}

func (h CredentialHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/enforcement/credentials/check" {
		http.NotFound(w, r)
		return
	}
	if h.Key == "" || r.Header.Get("Authorization") != "Bearer "+h.Key {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		UserID int64 `json:"user_id"`
		Write  bool  `json:"write"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.UserID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request"})
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	d := CheckCredential(body.UserID, body.Write, now)
	writeJSON(w, http.StatusOK, map[string]any{
		"allow": d.Allowed,
		"code":  d.Code,
		"state": d.State,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
