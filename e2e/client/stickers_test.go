package e2e

import (
	"testing"
	"time"
)

func TestStickerSetUploadGetFile(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	c, cancel := contextTimeout(30 * time.Second)
	defer cancel()
	var docID int64
	var docLoc = struct {
		id, hash int64
		ref      []byte
	}{}
	err := waitUntil(c, time.Second, func() (bool, error) {
		doc, err := stickerDocumentCtx(c, a)
		if err != nil {
			if _, ok := asRPCError(err); ok && serverError(err) {
				return false, err
			}
			if _, ok := asRPCError(err); !ok && err.Error() != "sticker sets have no documents yet" {
				return false, err
			}
			return false, nil
		}
		if doc == nil {
			return false, nil
		}
		docID = doc.GetId()
		docLoc.id = doc.GetId()
		docLoc.hash = doc.GetAccessHash()
		docLoc.ref = append([]byte(nil), doc.GetFileReference()...)
		return docID != 0, nil
	})
	if err != nil || docID == 0 {
		t.Fatalf("no sticker document from messages.getStickerSet: %v", err)
	}
	fileCtx, fileCancel := contextTimeout(10 * time.Second)
	defer fileCancel()
	res, err := a.Invoke(fileCtx, getFile(&documentRef{id: docLoc.id, hash: docLoc.hash, ref: docLoc.ref}))
	if err != nil {
		t.Fatalf("upload.getFile for sticker %d: %v", docID, err)
	}
	if len(bytesOf(res)) == 0 {
		t.Fatalf("upload.getFile returned %T with no bytes", res)
	}
}
