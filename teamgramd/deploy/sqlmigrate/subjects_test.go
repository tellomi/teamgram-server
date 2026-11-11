package sqlmigrate

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/report"
)

// checkSubjectsByUsers runs the by-users lookup against the migrated
// teamgram schema. go test ./... skips it; the compat workflow does not.
func checkSubjectsByUsers(t *testing.T, db *sql.DB, port string) {
	t.Helper()
	const (
		live    int64 = 910001
		bare    int64 = 910002
		gone    int64 = 910003
		held    int64 = 910004
		missing int64 = 910005
	)
	liveSub := bytes.Repeat([]byte{7}, 16)
	goneSub := bytes.Repeat([]byte{8}, 16)
	heldSub := bytes.Repeat([]byte{9}, 16)
	ids := []int64{live, bare, gone, held}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = db.Exec(`DELETE FROM users WHERE id=?`, id)
			_, _ = db.Exec(`DELETE FROM discover_subject WHERE user_id=?`, id)
		}
	})
	for _, id := range ids {
		if _, err := db.Exec(`DELETE FROM discover_subject WHERE user_id=?`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO users (id, access_hash, country_code, deleted, restricted) VALUES (?,?,?,?,?),(?,?,?,?,?),(?,?,?,?,?),(?,?,?,?,?)`,
		live, 1, "CN", 0, 0,
		bare, 1, "CN", 0, 0,
		gone, 1, "CN", 1, 0,
		held, 1, "CN", 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO discover_subject (sub, user_id) VALUES (?,?),(?,?),(?,?)`,
		liveSub, live, goneSub, gone, heldSub, held); err != nil {
		t.Fatal(err)
	}

	dsn := fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/teamgram?parseTime=true&charset=utf8mb4", port)
	svc, err := report.OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte("k"), 32)
	h := report.Handler{Service: svc, Key: key}
	body := fmt.Sprintf(`{"user_ids":[%d,%d,%d,%d,%d]}`, live, bare, gone, held, missing)
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/reports/subjects/by-users", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(key))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("by-users %d %s", rec.Code, rec.Body.String())
	}
	var parsed struct {
		Users []struct {
			UserID int64  `json:"user_id"`
			Status string `json:"status"`
			Sub    string `json:"sub"`
		} `json:"users"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	wantSub := base64.RawURLEncoding.EncodeToString(liveSub)
	want := []struct {
		id     int64
		status string
		sub    string
	}{
		{live, "mapped", wantSub},
		{bare, "empty", ""},
		{gone, "closed", ""},
		{held, "closed", ""},
		{missing, "not_found", ""},
	}
	if len(parsed.Users) != len(want) {
		t.Fatalf("by-users %+v", parsed.Users)
	}
	for i, item := range want {
		got := parsed.Users[i]
		if got.UserID != item.id || got.Status != item.status || got.Sub != item.sub {
			t.Fatalf("user %d %+v", i, got)
		}
	}
}
