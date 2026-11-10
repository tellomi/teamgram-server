package e2e

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMediaAccessHash is the production bug: an unrelated user downloaded
// a private photo and video with access_hash=1, resent the photo, and
// recovered the document from its sha256. The owner, the recipient, and
// the recipient of a forward still download. A sticker download still works.
func TestMediaAccessHash(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "access_a", "AccessA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "access_b", "AccessB")
	c, _ := loginNamed(t, cfg, ProfileIOS, "access_c", "AccessC")
	d, _ := loginNamed(t, cfg, ProfileAndroid, "access_d", "AccessD")

	peerOf := func(from *Client, name, first string) *mtproto.InputPeer {
		t.Helper()
		ph, err := cfg.Phone(name)
		if err != nil {
			t.Fatal(err)
		}
		cx, cancel := ctx()
		defer cancel()
		id, hash, err := from.ImportPhone(cx, ph.Number, first)
		if err != nil {
			t.Fatal(err)
		}
		return inputPeerUser(id, hash)
	}
	aToB := peerOf(a, "access_b", "AccessB")
	bToA := peerOf(b, "access_a", "AccessA")
	bToC := peerOf(b, "access_c", "AccessC")
	cToB := peerOf(c, "access_b", "AccessB")

	payload := bytes.Repeat([]byte("tellomi-access-video-v1\n"), 400)
	sum := sha256.Sum256(payload)
	doc, _ := sendPayload(t, a, aToB, "video/mp4", "private.mp4", payload)
	if !bytes.Equal(waitFile(t, a, docLocation(doc), payload), payload) {
		t.Fatal("owner download failed")
	}
	bobDoc := waitHistoryDocument(t, b, bToA, doc.GetId())
	if !bytes.Equal(waitFile(t, b, docLocation(bobDoc), payload), payload) {
		t.Fatal("recipient download failed")
	}

	forward(t, b, bToA, doc.GetId(), bToC)
	carolDoc := waitHistoryDocument(t, c, cToB, doc.GetId())
	if carolDoc.GetAccessHash() == 0 || carolDoc.GetAccessHash() != doc.GetAccessHash() {
		t.Fatalf("forward access_hash %d, sent %d", carolDoc.GetAccessHash(), doc.GetAccessHash())
	}
	if !bytes.Equal(waitFile(t, c, docLocation(carolDoc), payload), payload) {
		t.Fatal("forward recipient download failed")
	}

	jpeg := testJPEG(t, 320, 240, 3)
	photo, _ := sendPhotoBytes(t, a, aToB, "private.jpg", jpeg)
	size := largestPhotoSize(t, photo)
	if len(waitNonEmpty(t, a, photoLocation(photo, size))) == 0 {
		t.Fatal("owner photo download failed")
	}
	bobPhoto := waitHistoryPhoto(t, b, bToA, photo.GetId())
	if len(waitNonEmpty(t, b, photoLocation(bobPhoto, size))) == 0 {
		t.Fatal("recipient photo download failed")
	}

	badDoc := mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
		Id: doc.GetId(), AccessHash: 1, ThumbSize: "",
	}).To_InputFileLocation()
	expectRPC(t, d, &mtproto.TLUploadGetFile{
		Constructor: mtproto.CRC32_upload_getFile_be5335be,
		Location:    badDoc,
		Limit:       512 * 1024,
	}, "FILE_ID_INVALID")
	expectRPC(t, d, &mtproto.TLUploadGetFileHashes{
		Constructor:  mtproto.CRC32_upload_getFileHashes_9156982a,
		Location:     badDoc,
		Offset_INT64: 0,
	}, "FILE_ID_INVALID")
	badPhoto := mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
		Id: photo.GetId(), AccessHash: 1, ThumbSize: size,
	}).To_InputFileLocation()
	expectRPC(t, d, &mtproto.TLUploadGetFile{
		Constructor: mtproto.CRC32_upload_getFile_be5335be,
		Location:    badPhoto,
		Limit:       512 * 1024,
	}, "FILE_ID_INVALID")
	expectRPC(t, d, &mtproto.TLUploadGetFileHashes{
		Constructor: mtproto.CRC32_upload_getFileHashes_9156982a,
		Location:    badPhoto,
	}, "FILE_ID_INVALID")

	expectRPC(t, d, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        inputPeerSelf(),
		RandomId:    randomID(),
		Media: mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
			Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
				Id: photo.GetId(), AccessHash: 1,
			}).To_InputPhoto(),
		}).To_InputMedia(),
	}, "MEDIA_INVALID")
	expectRPC(t, d, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        inputPeerSelf(),
		RandomId:    randomID(),
		Media: mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
			Id_INPUTDOCUMENT: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
				Id: doc.GetId(), AccessHash: 1,
			}).To_InputDocument(),
		}).To_InputMedia(),
	}, "MEDIA_INVALID")

	byHash := documentFromRPC(invokeOK(t, d, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sum[:],
		Size2_INT64: int64(len(payload)),
		MimeType:    "video/mp4",
	}))
	if byHash.GetPredicateName() != mtproto.Predicate_documentEmpty || byHash.GetId() != 0 {
		t.Fatalf("stranger getDocumentByHash %+v", byHash)
	}
	own := documentFromRPC(invokeOK(t, a, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sum[:],
		Size2_INT64: int64(len(payload)),
		MimeType:    "video/mp4",
	}))
	if own.GetId() != doc.GetId() || own.GetAccessHash() != doc.GetAccessHash() {
		t.Fatalf("owner getDocumentByHash id %d hash %d", own.GetId(), own.GetAccessHash())
	}

	cx, cancel := ctx()
	defer cancel()
	sticker, err := stickerDocumentCtx(cx, d)
	if err != nil || sticker.GetId() == 0 {
		t.Fatalf("sticker document: %v", err)
	}
	res, err := d.Invoke(cx, &mtproto.TLUploadGetFile{
		Constructor: mtproto.CRC32_upload_getFile_be5335be,
		Location:    docLocation(sticker),
		Limit:       128 * 1024,
	})
	if err != nil || len(bytesOf(res)) == 0 {
		t.Fatalf("sticker download %v bytes %d", err, len(bytesOf(res)))
	}
}

func waitHistoryDocument(t *testing.T, c *Client, peer *mtproto.InputPeer, id int64) *mtproto.Document {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		hist := invokeOK(t, c, &mtproto.TLMessagesGetHistory{
			Constructor: mtproto.CRC32_messages_getHistory,
			Peer:        peer,
			Limit:       50,
		})
		for _, m := range allMessages(hist) {
			if doc := m.GetMedia().GetDocument(); doc != nil && doc.GetId() == id {
				return doc
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("document %d not in history", id)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func waitHistoryPhoto(t *testing.T, c *Client, peer *mtproto.InputPeer, id int64) *mtproto.Photo {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		hist := invokeOK(t, c, &mtproto.TLMessagesGetHistory{
			Constructor: mtproto.CRC32_messages_getHistory,
			Peer:        peer,
			Limit:       50,
		})
		for _, m := range allMessages(hist) {
			if p := m.GetMedia().GetPhoto_FLAGPHOTO(); p != nil && p.GetId() == id {
				return p
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("photo %d not in history", id)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func expectRPC(t *testing.T, c *Client, req mtproto.TLObject, message string) {
	t.Helper()
	cx, cancel := ctx()
	defer cancel()
	_, err := c.Invoke(cx, req)
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != message {
		t.Fatalf("%T err %v, want %s", req, err, message)
	}
}
