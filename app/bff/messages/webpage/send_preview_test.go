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
	if msg.GetMedia() != nil {
		t.Fatal("empty fetch should drop the card")
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
