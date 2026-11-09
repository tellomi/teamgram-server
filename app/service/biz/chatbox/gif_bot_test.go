package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestGifBotInlineRoundTrip(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	got, err := b.InlineResults(ctx, 3, userIn(mtproto.BotGifId), peerIn(9), "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetGallery() || len(got.GetResults()) < 4 || got.GetUsers()[0].GetId() != mtproto.BotGifId || !got.GetUsers()[0].GetBot() {
		t.Fatalf("results %d gallery %v", len(got.GetResults()), got.GetGallery())
	}
	hit := got.GetResults()[0]
	if hit.GetDocument().GetMimeType() != "video/mp4" || len(hit.GetDocument().GetThumbs()) == 0 {
		t.Fatal("inline gif has no playable document")
	}
	req, ready, err := b.TakeInlineResult(ctx, 3, peerIn(9), got.GetQueryId(), hit.GetId(), 11, false, false, false, 0, nil)
	if err != nil || ready != nil {
		t.Fatalf("send %v %v", err, ready)
	}
	doc := req.Message.GetMedia().GetDocument()
	if doc.GetId() != hit.GetDocument().GetId() || doc.GetMimeType() != "video/mp4" {
		t.Fatalf("sent media %#v", req.Message.GetMedia())
	}
	filtered, err := b.InlineResults(ctx, 3, userIn(mtproto.BotGifId), peerIn(9), "purple", "", nil)
	if err != nil || len(filtered.GetResults()) != 1 {
		t.Fatalf("filter %d %v", len(filtered.GetResults()), err)
	}
}
