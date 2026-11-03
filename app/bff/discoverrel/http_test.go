package discoverrel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestRelationsLiveContactsAndCloseFriends(t *testing.T) {
	names := subs(t, 1, 2, 3, 4)
	author, contact, friend, stranger := names[0], names[1], names[2], names[3]
	mem := NewMemory()
	key := bytesSeq(9, 32)
	for _, id := range []int64{1, 2, 3, 4} {
		if err := mem.Save(context.Background(), id, discoverauth.Pseudonym(key, id)); err != nil {
			t.Fatal(err)
		}
	}
	// Author 1 lists 2 as a contact and 3 as a close friend. 4 is absent.
	// Viewer 2's own contacts must not make 1 visible.
	mem.PutEdge(1, 2, false)
	mem.PutEdge(1, 3, true)
	mem.PutEdge(2, 1, true)

	h := Handler{Dir: mem, Key: bytesSeq(4, 32)}
	body := `{"viewer":"` + contact + `","authors":["` + author + `","` + friend + `"]}`
	rec := call(t, h, body, h.Key)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var ans Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if !ans.ViewerKnown || len(ans.Results) != 2 {
		t.Fatalf("%+v", ans)
	}
	if !ans.Results[0].Contact || ans.Results[0].CloseFriend || ans.Results[0].Author != author {
		t.Fatalf("contact %+v", ans.Results[0])
	}
	// friend is an author here, and the viewer is the plain contact, who is
	// not in author-friend's list. The second author is user 3.
	if ans.Results[1].Contact || ans.Results[1].CloseFriend {
		t.Fatalf("stranger author %+v", ans.Results[1])
	}
	// User 2 lists user 1 as a close friend, so user 1 can see user 2.
	// That row is user 2's list, not user 1's.
	rec = call(t, h, `{"viewer":"`+author+`","authors":["`+contact+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if !ans.Results[0].Contact || !ans.Results[0].CloseFriend {
		t.Fatalf("author list %+v", ans.Results[0])
	}

	body = `{"viewer":"` + friend + `","authors":["` + author + `","` + stranger + `","` + friend + `"]}`
	rec = call(t, h, body, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if !ans.Results[0].Contact || !ans.Results[0].CloseFriend {
		t.Fatalf("close friend %+v", ans.Results[0])
	}
	if ans.Results[1].Known && (ans.Results[1].Contact || ans.Results[1].CloseFriend) {
		t.Fatalf("stranger %+v", ans.Results[1])
	}
	if ans.Results[1].Known != true || ans.Results[1].Contact {
		t.Fatalf("known stranger %+v", ans.Results[1])
	}
	if ans.Results[2].Contact || ans.Results[2].CloseFriend {
		t.Fatalf("self %+v", ans.Results[2])
	}

	// Dropping the contact hides the old post immediately.
	delete(mem.edges[1], 2)
	rec = call(t, h, `{"viewer":"`+contact+`","authors":["`+author+`"]}`, h.Key)
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.Results[0].Contact || ans.Results[0].CloseFriend {
		t.Fatalf("removed contact %+v", ans.Results[0])
	}
}

func TestRelationsAuthAndShape(t *testing.T) {
	h := Handler{Dir: NewMemory(), Key: bytesSeq(4, 32)}
	names := subs(t, 1)
	if rec := call(t, h, `{"viewer":"`+names[0]+`","authors":["`+names[0]+`"]}`, bytesSeq(5, 32)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth %d", rec.Code)
	}
	if rec := call(t, h, `{"viewer":"abc","authors":["abc"]}`, h.Key); rec.Code != http.StatusBadRequest {
		t.Fatalf("sub %d", rec.Code)
	}
	if rec := call(t, h, `{"viewer":"`+names[0]+`","authors":[]}`, h.Key); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty %d", rec.Code)
	}
	unknown := discoverauth.EncodeSub(bytesSeq(1, 16))
	rec := call(t, h, `{"viewer":"`+unknown+`","authors":["`+names[0]+`"]}`, h.Key)
	var ans Answer
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.ViewerKnown || ans.Results[0].Known || ans.Results[0].Contact {
		t.Fatalf("%+v", ans)
	}
}

func call(t *testing.T, h Handler, body string, key []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/relations/check", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(key))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRememberNoopWhenOff(t *testing.T) {
	UseDirectory(nil)
	if err := Remember(context.Background(), 1, bytesSeq(0, 16)); err != nil {
		t.Fatal(err)
	}
}
