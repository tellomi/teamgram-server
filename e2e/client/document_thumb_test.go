package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestCustomDocumentThumbIsStored covers ISSUE-S-DOC-THUMB-001 (row 153).
// A file that is not itself an image keeps the JPEG the client attached.
func TestCustomDocumentThumbIsStored(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610441", "ThumbA")
	_, _ = dialPhone(t, ctx, cfg, "9996610442", "ThumbB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610442", "ThumbB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bID, bHash)
	fileID := uploadBytes(t, a, []byte("%PDF-1.4 custom-thumb"))
	thumbID := uploadBytes(t, a, testJPEG(t, 80, 60, 7))
	res, err := a.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        peer,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "notes.pdf",
			}).To_InputFile(),
			Thumb: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: thumbID,
				Parts:    1,
				Name:     "notes.jpg",
			}).To_InputFile(),
			MimeType: "application/pdf",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: "notes.pdf"}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
		Message:  "notes",
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc *mtproto.Document
	for _, m := range allMessages(res) {
		if d := m.GetMedia().GetDocument(); d != nil && d.GetId() != 0 {
			doc = d
			break
		}
	}
	if doc == nil {
		t.Fatalf("sendMedia returned %T", res)
	}
	var stripped, medium bool
	for _, sz := range doc.GetThumbs() {
		switch sz.GetType() {
		case mtproto.PhotoSZStrippedType:
			stripped = len(sz.GetBytes()) > 0
		case mtproto.PhotoSZMediumType:
			medium = sz.GetW() > 0 && sz.GetH() > 0
		}
	}
	if !stripped || !medium {
		t.Fatalf("document thumbs %#v", doc.GetThumbs())
	}
}
