package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestChooseVideoSizeKeepsPortraitAndDropsSquare(t *testing.T) {
	// Client measured the picture. Probe only saw the coded landscape
	// frame and did not report a display matrix.
	w, h := chooseVideoSize(1080, 1920, 1920, 1080, false, false)
	if w != 1080 || h != 1920 {
		t.Fatalf("kept probe landscape %dx%d", w, h)
	}

	// Client sent the thumb's square. The file is tall.
	w, h = chooseVideoSize(320, 320, 1080, 1920, false, false)
	if w != 1080 || h != 1920 {
		t.Fatalf("square client %dx%d", w, h)
	}

	// Display matrix says the coded landscape frame is portrait.
	w, h = chooseVideoSize(1920, 1080, 1080, 1920, false, true)
	if w != 1080 || h != 1920 {
		t.Fatalf("rotated probe %dx%d", w, h)
	}

	// Round video stays on the client square.
	w, h = chooseVideoSize(400, 400, 1080, 1920, true, true)
	if w != 400 || h != 400 {
		t.Fatalf("round %dx%d", w, h)
	}

	// Agreeing portrait sizes stay portrait.
	w, h = chooseVideoSize(720, 1280, 720, 1280, false, false)
	if w != 720 || h != 1280 {
		t.Fatalf("agree %dx%d", w, h)
	}
}

func TestFitAspectNeverSquaresATallVideo(t *testing.T) {
	w, h := fitAspect(720, 1280, 40)
	if w <= 0 || h <= w || w == h {
		t.Fatalf("tall %dx%d", w, h)
	}
	w, h = fitAspect(1920, 1080, 40)
	if h <= 0 || w <= h {
		t.Fatalf("wide %dx%d", w, h)
	}
	w, h = fitAspect(100, 100, 40)
	if w != 40 || h != 40 {
		t.Fatalf("square %dx%d", w, h)
	}
}

func TestFinishGifDocumentDoesNotSquareTallVideo(t *testing.T) {
	doc := mtproto.MakeTLDocument(&mtproto.Document{
		Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
				W: 720, H: 1280,
			}).To_DocumentAttribute(),
		},
	}).To_Document()
	got := finishGifDocument(doc, []byte("mp4"))
	if len(got.GetThumbs()) != 1 {
		t.Fatalf("thumbs %d", len(got.GetThumbs()))
	}
	sz := got.GetThumbs()[0]
	if sz.GetW() <= 0 || sz.GetH() <= sz.GetW() || (sz.GetW() == 40 && sz.GetH() == 40) {
		t.Fatalf("placeholder %dx%d", sz.GetW(), sz.GetH())
	}
	if len(got.GetVideoThumbs()) != 1 || got.GetVideoThumbs()[0].GetW() != 720 || got.GetVideoThumbs()[0].GetH() != 1280 {
		t.Fatalf("video thumb %#v", got.GetVideoThumbs())
	}
}

func TestMakeMp4DocumentAttributesUsesDisplaySize(t *testing.T) {
	in := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			W: 320, H: 320, Duration_FLOAT64: 3,
		}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
			FileName: "clip.mp4",
		}).To_DocumentAttribute(),
	}
	out := makeMp4DocumentAttributes(in, 1080, 1920)
	var video *mtproto.DocumentAttribute
	for _, attr := range out {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeVideo {
			video = attr
		}
	}
	if video == nil || video.GetW() != 1080 || video.GetH() != 1920 || !video.GetSupportsStreaming() {
		t.Fatalf("attribute %#v", video)
	}
}
