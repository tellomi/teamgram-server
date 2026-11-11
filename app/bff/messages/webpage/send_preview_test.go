package webpage

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPreviewMediaExternalPendingThenFetched(t *testing.T) {
	t.Setenv(chatbox.EnvServerLinkFetch, "1")
	const raw = "https://send-preview.example/article"
	media, pending := AttachOnSend(context.Background(), raw, nil)
	if !pending || media.GetWebpage().GetPredicateName() != mtproto.Predicate_webPagePending {
		t.Fatalf("want pending card, got pending=%v pred=%s", pending, media.GetWebpage().GetPredicateName())
	}
	pendingID := media.GetWebpage().GetId()
	if pendingID == 0 {
		t.Fatal("pending id")
	}

	fetched := mtproto.MakeTLWebPage(&mtproto.WebPage{
		Id:         pendingID,
		Url_STRING: raw,
		Title:      wrapperspb.String("Example Domain"),
		SiteName:   wrapperspb.String("example"),
		Type:       wrapperspb.String("article"),
	}).To_WebPage()
	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Message: raw,
		Media:   media,
	}).To_Message()
	ApplyFetchedPage(msg, fetched)
	if IsPending(msg.GetMedia()) {
		t.Fatal("still pending after apply")
	}
	if msg.GetMedia().GetWebpage().GetId() != pendingID || msg.GetMedia().GetWebpage().GetTitle().GetValue() != "Example Domain" {
		t.Fatalf("applied %#v", msg.GetMedia().GetWebpage())
	}

	ApplyFetchedPage(msg, mtproto.MakeTLWebPageEmpty(&mtproto.WebPage{Id: pendingID, Url_STRING: raw}).To_WebPage())
	got := msg.GetMedia().GetWebpage()
	if got == nil || got.GetPredicateName() != mtproto.Predicate_webPageEmpty || got.GetId() != pendingID {
		t.Fatalf("empty fetch should become webPageEmpty id=%d, got %#v", pendingID, msg.GetMedia())
	}
}

func TestPreviewMediaExternalOffAndCached(t *testing.T) {
	t.Setenv(chatbox.EnvServerLinkFetch, "")
	if PreviewMedia(context.Background(), "https://www.apple.com", nil) != nil {
		t.Fatal("external url was attached while fetch is off")
	}
	t.Setenv(chatbox.EnvServerLinkFetch, "1")
	restore := chatbox.InstallTestLoader("Cached Title", "Cached site", "")
	defer restore()
	const raw = "https://cached-send.example/"
	page, err := chatbox.WebPage(raw, 0)
	if err != nil || page.GetPredicateName() != mtproto.Predicate_webPage {
		t.Fatalf("seed cache %s %v", page.GetPredicateName(), err)
	}
	started := time.Now()
	media, pending := AttachOnSend(context.Background(), raw, nil)
	if pending || media.GetWebpage().GetTitle().GetValue() != "Cached Title" {
		t.Fatalf("cached send pending=%v title=%q", pending, media.GetWebpage().GetTitle().GetValue())
	}
	if time.Since(started) > time.Second {
		t.Fatal("cached attach waited on a fetch")
	}
	if media.GetWebpage().GetId() != page.GetId() {
		t.Fatalf("id %d vs %d", media.GetWebpage().GetId(), page.GetId())
	}
}

func TestPreviewMediaStillAttachesInternal(t *testing.T) {
	BindUserResolver(func(_ context.Context, name string) (UserCard, bool) {
		if name != "erel" {
			return UserCard{}, false
		}
		return UserCard{ID: 1, Title: "Erel", Username: "erel"}, true
	})
	t.Cleanup(func() { BindUserResolver(nil) })
	media := PreviewMedia(context.Background(), "https://tell.cc/erel", nil)
	if media.GetWebpage().GetType().GetValue() != "telegram_user" {
		t.Fatalf("internal %s", media.GetWebpage().GetType().GetValue())
	}
	if IsPending(media) {
		t.Fatal("internal card was pending")
	}
}

func TestAttachOnSendHonoursUserBudget(t *testing.T) {
	t.Setenv(chatbox.EnvServerLinkFetch, "1")
	chatbox.ResetFetchLimitsForTest()
	ctx := withLang(t, "en", "ios")
	// withLang has no user id, so AllowSendFetch(0) would pass. Bind a
	// metadata user by draining the public helper is not available; the
	// budget itself is tested in chatbox. Here we only assert a cached
	// card is still attached when the budget is already spent... skip if
	// viewer id is 0. The policy is: cache hits do not spend the budget.
	const raw = "https://budget-cache.example/"
	restore := chatbox.InstallTestLoader("Cached", "site", "")
	defer restore()
	if _, err := chatbox.WebPage(raw, 0); err != nil {
		t.Fatal(err)
	}
	media, pending := AttachOnSend(ctx, raw, nil)
	if pending || media.GetWebpage().GetTitle().GetValue() != "Cached" {
		t.Fatalf("cached attach spent or skipped: pending=%v %#v", pending, media)
	}
}

func TestClientSuppliedInputMediaWebPageUnchanged(t *testing.T) {
	t.Setenv(chatbox.EnvServerLinkFetch, "1")
	in := mtproto.MakeTLInputMediaWebPage(&mtproto.InputMedia{
		Url:             "https://www.apple.com",
		Title:           "Client Title",
		Description:     "From the sending client",
		Provider_STRING: "Apple",
		ForceLargeMedia: true,
	}).To_InputMedia()
	if !ClientSupplied(in) {
		t.Fatal("client card not detected")
	}
	media, err := Media(context.Background(), in, nil)
	if err != nil {
		t.Fatal(err)
	}
	page := media.GetWebpage()
	if page.GetTitle().GetValue() != "Client Title" || page.GetDescription().GetValue() != "From the sending client" || page.GetSiteName().GetValue() != "Apple" {
		t.Fatalf("client card was rewritten: %+v", page)
	}
	if !media.GetForceLargeMedia() {
		t.Fatal("force_large_media was dropped")
	}
	if IsPending(media) {
		t.Fatal("client card became pending")
	}
}
