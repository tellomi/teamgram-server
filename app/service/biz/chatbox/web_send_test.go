package chatbox

import (
	"context"
	"net/url"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPendingAndFetchedShareID(t *testing.T) {
	restore := stubWebFetch(t, pageMeta{Title: "Hello", Description: "From the page", Site: "Example"})
	defer restore()

	const raw = "https://pending-id.example/page"
	pending := PendingExternalPage(raw)
	if pending == nil || pending.GetPredicateName() != mtproto.Predicate_webPagePending || pending.GetId() == 0 {
		t.Fatalf("pending %#v", pending)
	}
	if _, ok := CachedExternalPage(context.Background(), raw); ok {
		t.Fatal("cache hit before fetch")
	}
	page, err := WebPage(raw, 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPage {
		t.Fatalf("fetch %s %v", page.GetPredicateName(), err)
	}
	if page.GetId() != pending.GetId() {
		t.Fatalf("id pending=%d fetched=%d", pending.GetId(), page.GetId())
	}
	cached, ok := CachedExternalPage(context.Background(), raw)
	if !ok || cached.GetId() != pending.GetId() || cached.GetTitle().GetValue() != "Hello" {
		t.Fatalf("cached %#v ok=%v", cached, ok)
	}
	if MediaFromPage(pending).GetWebpage().GetPredicateName() != mtproto.Predicate_webPagePending {
		t.Fatal("pending media")
	}
	if MediaFromPage(page).GetWebpage().GetTitle().GetValue() != "Hello" {
		t.Fatal("fetched media")
	}
	if MediaFromPage(emptyPage(mustParse(t, raw))) != nil {
		t.Fatal("empty page became a card")
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := parseWebURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
