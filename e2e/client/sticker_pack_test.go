package e2e

import (
	"bytes"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// tinyStickerWEBP is an 8×8 lossless WebP. iOS sticker creation uploads
// image/webp named sticker.webp, then stickers.createStickerSet with short_name "".
var tinyStickerWEBP = []byte{
	0x52, 0x49, 0x46, 0x46, 0x1c, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x4c,
	0x10, 0x00, 0x00, 0x00, 0x2f, 0x07, 0xc0, 0x01, 0x00, 0x07, 0x10, 0xfd, 0x2f, 0x54, 0xff, 0x03,
	0x11, 0xd1, 0xff, 0x00,
}

func TestCreateStickerPack(t *testing.T) {
	cfg := requireE2E(t)
	owner, self := loginNamed(t, cfg, ProfileIOS, "sticker_owner", "Sticker")
	other, _ := loginNamed(t, cfg, ProfileIOS, "sticker_other", "Other")

	doc := uploadStickerDocument(t, owner, "image/webp", "sticker.webp", tinyStickerWEBP)
	if doc.GetId() == 0 || doc.GetAccessHash() == 0 {
		t.Fatalf("upload %#v", doc)
	}
	if doc.GetMimeType() != "image/webp" || doc.GetFileReference() == nil {
		t.Fatalf("uploaded document mime=%s ref=%v", doc.GetMimeType(), doc.GetFileReference())
	}

	created := createStickerSet(t, owner, self, "", []*mtproto.InputStickerSetItem{stickerItem(doc, "🫥")})
	set := created.GetSet()
	if set == nil || set.GetId() == 0 || set.GetAccessHash() == 0 || set.GetShortName() == "" || set.GetCount() != 1 {
		t.Fatalf("set %#v", set)
	}
	if len(created.GetDocuments()) != 1 || created.GetDocuments()[0].GetId() != doc.GetId() {
		t.Fatalf("documents %#v", created.GetDocuments())
	}
	out := created.GetDocuments()[0]
	if out.GetId() == 0 || out.GetFileReference() == nil || out.GetDcId() == 0 || out.GetMimeType() != "image/webp" {
		t.Fatalf("set document id=%d mime=%s dc=%d", out.GetId(), out.GetMimeType(), out.GetDcId())
	}
	if !hasStickerAttr(out, "🫥") {
		t.Fatalf("attributes %#v", out.GetAttributes())
	}

	got := invokeOK(t, owner, &mtproto.TLMessagesGetStickerSet{
		Constructor: mtproto.CRC32_messages_getStickerSet_c8a0ec74,
		Stickerset: mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
			ShortName: set.GetShortName(),
		}).To_InputStickerSet(),
	})
	again, ok := got.(*mtproto.TLMessagesStickerSet)
	if !ok || again.GetSet().GetId() != set.GetId() || len(again.GetDocuments()) != 1 {
		t.Fatalf("getStickerSet %T %#v", got, got)
	}

	fileCtx, cancel := contextTimeout(15 * time.Second)
	defer cancel()
	var body []byte
	err := waitUntil(fileCtx, 500*time.Millisecond, func() (bool, error) {
		res, err := owner.Invoke(fileCtx, getFile(&documentRef{
			id:   out.GetId(),
			hash: out.GetAccessHash(),
			ref:  append([]byte(nil), out.GetFileReference()...),
		}))
		if err != nil {
			if _, ok := asRPCError(err); ok && serverError(err) {
				return false, err
			}
			return false, nil
		}
		body = bytesOf(res)
		return len(body) > 0, nil
	})
	if err != nil || !bytes.Equal(body, tinyStickerWEBP) {
		t.Fatalf("getFile len=%d err=%v", len(body), err)
	}

	plain := uploadStickerDocument(t, owner, "application/octet-stream", "note.bin", []byte("not-a-sticker"))
	_, err = invoke(t, owner, &mtproto.TLStickersCreateStickerSet{
		Constructor: mtproto.CRC32_stickers_createStickerSet,
		UserId:      inputUser(self.ID, self.AccessHash),
		Title:       "Paper",
		ShortName:   "",
		Stickers:    []*mtproto.InputStickerSetItem{stickerItem(plain, "😀")},
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "STICKER_MIME_INVALID" {
		t.Fatalf("bad media: %v", err)
	}

	_, err = invoke(t, owner, &mtproto.TLStickersCreateStickerSet{
		Constructor: mtproto.CRC32_stickers_createStickerSet,
		UserId:      inputUser(self.ID, self.AccessHash),
		Title:       "Missing",
		ShortName:   "",
		Stickers: []*mtproto.InputStickerSetItem{
			stickerItem(&mtproto.Document{Id: randomID(), AccessHash: randomID(), FileReference: []byte{}}, "😀"),
		},
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "STICKER_DOCUMENT_INVALID" {
		t.Fatalf("missing document: %v", err)
	}

	fileID := randomID()
	_, err = invoke(t, owner, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       tinyStickerWEBP,
	})
	if err != nil {
		t.Fatalf("save part: %v", err)
	}
	_, err = invoke(t, owner, stickerUpload(fileID, 2, "image/webp", "sticker.webp"))
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "FILE_PART_1_MISSING" {
		t.Fatalf("missing part: %v", err)
	}

	_, err = invoke(t, other, &mtproto.TLStickersDeleteStickerSet{
		Constructor: mtproto.CRC32_stickers_deleteStickerSet,
		Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{
			Id:         set.GetId(),
			AccessHash: set.GetAccessHash(),
		}).To_InputStickerSet(),
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 400 || rpc.Message != "STICKERSET_INVALID" {
		t.Fatalf("other user delete: %v", err)
	}
}

func uploadStickerDocument(t *testing.T, c *Client, mime, name string, body []byte) *mtproto.Document {
	t.Helper()
	fileID := randomID()
	if _, err := invoke(t, c, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       body,
	}); err != nil {
		t.Fatal(err)
	}
	res := invokeOK(t, c, stickerUpload(fileID, 1, mime, name))
	doc := documentFromMedia(res)
	if doc == nil || doc.GetId() == 0 {
		t.Fatalf("uploadMedia %T", res)
	}
	return doc
}

func stickerUpload(fileID int64, parts int32, mime, name string) mtproto.TLObject {
	return &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    parts,
				Name:     name,
			}).To_InputFile(),
			MimeType: mime,
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
				mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{W: 512, H: 512}).To_DocumentAttribute(),
				mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{
					Alt:        "",
					Stickerset: mtproto.MakeTLInputStickerSetEmpty(nil).To_InputStickerSet(),
				}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
	}
}

func createStickerSet(t *testing.T, c *Client, self Self, short string, stickers []*mtproto.InputStickerSetItem) *mtproto.TLMessagesStickerSet {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLStickersCreateStickerSet{
		Constructor: mtproto.CRC32_stickers_createStickerSet,
		UserId:      inputUser(self.ID, self.AccessHash),
		Title:       "Mine",
		ShortName:   short,
		Stickers:    stickers,
	})
	set, ok := res.(*mtproto.TLMessagesStickerSet)
	if !ok || set.GetSet() == nil {
		t.Fatalf("createStickerSet %T", res)
	}
	return set
}

func stickerItem(doc *mtproto.Document, emoji string) *mtproto.InputStickerSetItem {
	return mtproto.MakeTLInputStickerSetItem(&mtproto.InputStickerSetItem{
		Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:            doc.GetId(),
			AccessHash:    doc.GetAccessHash(),
			FileReference: append([]byte(nil), doc.GetFileReference()...),
		}).To_InputDocument(),
		Emoji: emoji,
	}).To_InputStickerSetItem()
}

func documentFromMedia(obj mtproto.TLObject) *mtproto.Document {
	switch m := obj.(type) {
	case *mtproto.TLMessageMediaDocument:
		return m.GetDocument()
	default:
		return nil
	}
}

func hasStickerAttr(doc *mtproto.Document, alt string) bool {
	for _, attr := range doc.GetAttributes() {
		if attr.GetPredicateName() == mtproto.Predicate_documentAttributeSticker && attr.GetAlt() == alt {
			return true
		}
	}
	return false
}

func invoke(t *testing.T, c *Client, req mtproto.TLObject) (mtproto.TLObject, error) {
	t.Helper()
	cx, cancel := ctx()
	defer cancel()
	return c.Invoke(cx, req)
}
