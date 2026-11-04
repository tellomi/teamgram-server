package discoverprofile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/discoverauth"
)

func subs(t *testing.T, ids ...int64) []string {
	t.Helper()
	key := bytesSeq(9, 32)
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = discoverauth.EncodeSub(discoverauth.Pseudonym(key, id))
	}
	return out
}

func bytesSeq(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}

func seed(t *testing.T) (*Memory, []string) {
	t.Helper()
	names := subs(t, 1, 2, 3, 4)
	mem := NewMemory()
	key := bytesSeq(9, 32)
	for _, id := range []int64{1, 2, 3, 4} {
		mem.Save(id, discoverauth.Pseudonym(key, id))
		mem.PutProfile(id, Profile{
			FirstName: "用户" + string(rune('0'+id)),
			Username:  "u" + string(rune('0'+id)),
			About:     "简介",
			PhotoID:   8000 + id,
		})
	}
	// Author 1: photo and about are contacts-only. 2 is a contact, 3 is a close friend.
	mem.PutPrivacy(1, Privacy{
		Photo:    rules(mtproto.Predicate_privacyValueAllowContacts),
		PhotoSet: true,
		About:    rules(mtproto.Predicate_privacyValueAllowContacts),
		AboutSet: true,
	})
	mem.PutEdge(1, 2, false)
	mem.PutEdge(1, 3, true)
	// Author 2: photo is close friends only. 1 lists nobody here.
	mem.PutPrivacy(2, Privacy{
		Photo:    rules(mtproto.Predicate_privacyValueAllowCloseFriends),
		PhotoSet: true,
	})
	mem.PutEdge(2, 3, true)
	return mem, names
}

func TestResolveFiltersPhotoByContacts(t *testing.T) {
	mem, names := seed(t)
	author, contact, friend, stranger := names[0], names[1], names[2], names[3]
	h := Handler{Dir: mem, Key: bytesSeq(4, 32)}

	rec := call(t, h, `{"viewer":"`+stranger+`","authors":["`+author+`","`+friend+`"]}`, h.Key)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var ans Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if !ans.ViewerKnown || len(ans.Results) != 2 {
		t.Fatalf("%+v", ans)
	}
	if ans.Results[0].Author != author || ans.Results[0].Photo || ans.Results[0].PhotoID != "" || ans.Results[0].About != "" || ans.Results[0].FirstName == "" || ans.Results[0].Username == "" {
		t.Fatalf("stranger sees restricted card %+v", ans.Results[0])
	}
	if strings.Contains(rec.Body.String(), "8001") || strings.Contains(rec.Body.String(), "phone") {
		t.Fatalf("hidden photo id or phone leaked: %s", rec.Body.String())
	}
	// User 3 has no privacy row, so the stranger still sees that photo.
	if !ans.Results[1].Photo || ans.Results[1].PhotoID != "8003" || !ans.Results[1].AboutVisible {
		t.Fatalf("public card %+v", ans.Results[1])
	}

	rec = call(t, h, `{"viewer":"`+contact+`","authors":["`+author+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if !ans.Results[0].Photo || ans.Results[0].PhotoID != "8001" || ans.Results[0].About != "简介" {
		t.Fatalf("contact %+v", ans.Results[0])
	}

	rec = call(t, h, `{"viewer":"`+friend+`","authors":["`+author+`","`+contact+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if !ans.Results[0].Photo || ans.Results[0].PhotoID != "8001" {
		t.Fatalf("close friend of author %+v", ans.Results[0])
	}
	// User 2 shows a photo only to close friends. User 3 is one of them.
	if ans.Results[1].Author != contact || !ans.Results[1].Photo || ans.Results[1].PhotoID != "8002" {
		t.Fatalf("close friend of user 2 %+v", ans.Results[1])
	}

	rec = call(t, h, `{"viewer":"`+contact+`","authors":["`+contact+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	// User 2's photo is close-friends-only, but the viewer is user 2.
	if !ans.Results[0].Photo || ans.Results[0].PhotoID != "8002" {
		t.Fatalf("own close-friends photo %+v", ans.Results[0])
	}
	rec = call(t, h, `{"viewer":"`+author+`","authors":["`+contact+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.Results[0].Photo {
		t.Fatalf("plain contact saw close-friends photo %+v", ans.Results[0])
	}
}

func TestResolveDeletedAndUnknown(t *testing.T) {
	mem, names := seed(t)
	mem.PutProfile(4, Profile{FirstName: "已删", Username: "gone", About: "x", PhotoID: 1, Deleted: true})
	h := Handler{Dir: mem, Key: bytesSeq(4, 32)}
	unknown := discoverauth.EncodeSub(bytesSeq(1, 16))
	rec := call(t, h, `{"viewer":"`+unknown+`","authors":["`+names[3]+`","`+unknown+`"]}`, h.Key)
	var ans Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.ViewerKnown || !ans.Results[0].Deleted || ans.Results[0].FirstName != "" || ans.Results[0].Photo || ans.Results[1].Known {
		t.Fatalf("%+v", ans)
	}
	// Unknown viewer is a stranger, so author 1's contact-only photo stays hidden.
	rec = call(t, h, `{"viewer":"`+unknown+`","authors":["`+names[0]+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.Results[0].Photo || ans.Results[0].FirstName == "" {
		t.Fatalf("unknown viewer %+v", ans.Results[0])
	}
}

func TestResolveAuthAndErrors(t *testing.T) {
	mem, names := seed(t)
	h := Handler{Dir: mem, Key: bytesSeq(4, 32)}
	if rec := call(t, h, `{"viewer":"`+names[0]+`","authors":["`+names[0]+`"]}`, bytesSeq(5, 32)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth %d", rec.Code)
	}
	if rec := call(t, h, `{"viewer":"abc","authors":["abc"]}`, h.Key); rec.Code != http.StatusBadRequest {
		t.Fatalf("sub %d", rec.Code)
	}
	if rec := call(t, h, `{"viewer":"`+names[0]+`","authors":[]}`, h.Key); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty %d", rec.Code)
	}
	var many strings.Builder
	many.WriteString(`{"viewer":"` + names[0] + `","authors":[`)
	for i := 0; i < maxAuthors+1; i++ {
		if i > 0 {
			many.WriteByte(',')
		}
		many.WriteString(`"` + names[0] + `"`)
	}
	many.WriteString(`]}`)
	if rec := call(t, h, many.String(), h.Key); rec.Code != http.StatusBadRequest {
		t.Fatalf("too many %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, pathPost, nil)
	req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(h.Key))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/internal/v1/profiles/other", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("path %d", rec.Code)
	}
	mem.Fail(errors.New("down"))
	if rec := call(t, h, `{"viewer":"`+names[0]+`","authors":["`+names[1]+`"]}`, h.Key); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("down %d %s", rec.Code, rec.Body.String())
	}
}

func TestStartStaysOffWithoutKey(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	Start("user:pass@tcp(127.0.0.1:1)/teamgram")
	t.Setenv(EnvEnabled, "1")
	t.Setenv(EnvKey, "")
	Start("")
	if _, err := decodeKey("aaaa"); err == nil {
		t.Fatal("short key")
	}
}

func call(t *testing.T, h Handler, body string, key []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, pathPost, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(key))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
