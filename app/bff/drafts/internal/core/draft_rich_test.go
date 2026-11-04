package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestRichOnlyDraftIsStored(t *testing.T) {
	if richFromInput(nil) != nil {
		t.Fatal("nil")
	}
	if richFromInput(&mtproto.InputRichMessage{Rtl: true}) != nil {
		t.Fatal("rtl alone is not a draft")
	}
	got := richFromInput(&mtproto.InputRichMessage{Markdown: "hello **there**"})
	if got == nil || len(got.GetBlocks()) != 1 {
		t.Fatalf("markdown %#v", got)
	}
	plain := got.GetBlocks()[0].GetText().GetText_STRING()
	if plain != "hello **there**" {
		t.Fatalf("text %q", plain)
	}
	block := mtproto.MakeTLPageBlockParagraph(&mtproto.PageBlock{
		Text: mtproto.MakeTLTextPlain(&mtproto.RichText{Text_STRING: "block"}).To_RichText(),
	}).To_PageBlock()
	got = richFromInput(&mtproto.InputRichMessage{
		Blocks: []*mtproto.PageBlock{block},
		Photos: []*mtproto.InputPhoto{{Id: 9, AccessHash: 3}},
	})
	if got == nil || len(got.GetBlocks()) != 1 || len(got.GetPhotos()) != 1 || got.GetPhotos()[0].GetId() != 9 {
		t.Fatalf("blocks %#v", got)
	}
}
