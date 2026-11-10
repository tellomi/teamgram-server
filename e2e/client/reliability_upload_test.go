//go:build reliability

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestReliabilityUploadFailureRecovery covers two storage faults.
// A 5xx from the object store fails the upload RPC, leaves no blob row and
// no object, and the same bytes succeed once the store answers again.
// Killing dfs while a PUT is in flight fails that RPC; after dfs is back the
// same bytes upload and download.
func TestReliabilityUploadFailureRecovery(t *testing.T) {
	s := newReliabilityStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, _ := s.login(t, ProfileIOS, reliabilityPhone("0331"), "Upload")

	t.Run("object store 5xx", func(t *testing.T) {
		payload := solidJPEG(t, 7)
		sum := sha256.Sum256(payload)
		sha := hex.EncodeToString(sum[:])
		before := s.objectKeys(t)
		s.proxy.setMode(s3Fail)
		defer s.proxy.setMode(s3Pass)

		err := uploadJPEGErr(ctx, c, payload)
		assertRPC(t, err, 500, "INTERNAL_SERVER_ERROR")
		if n := s.scalar(t, "SELECT COUNT(*) FROM media_blobs WHERE sha256 = ?", sha); n != "0" {
			t.Fatalf("media_blobs rows after 5xx: %s", n)
		}
		if extra := addedKeys(before, s.objectKeys(t)); len(extra) != 0 {
			t.Fatalf("orphan objects after 5xx: %v", extra)
		}

		s.proxy.setMode(s3Pass)
		photo := uploadJPEG(t, ctx, c, payload)
		if n := s.scalar(t, "SELECT COUNT(*) FROM media_blobs WHERE sha256 = ?", sha); n != "1" {
			t.Fatalf("media_blobs rows after retry: %s", n)
		}
		if extra := addedKeys(before, s.objectKeys(t)); len(extra) == 0 {
			t.Fatal("retry wrote no object")
		}
		downloadPhoto(t, ctx, c, photo)
	})

	t.Run("document object store 5xx", func(t *testing.T) {
		payload := bytes.Repeat([]byte("reliability-doc-"), 1024)
		sum := sha256.Sum256(payload)
		sha := hex.EncodeToString(sum[:])
		before := s.objectKeys(t)
		s.proxy.setMode(s3Fail)
		defer s.proxy.setMode(s3Pass)

		_, err := uploadDocumentResult(ctx, c, payload)
		assertRPC(t, err, 500, "INTERNAL_SERVER_ERROR")
		if n := s.scalar(t, "SELECT COUNT(*) FROM media_blobs WHERE sha256 = ?", sha); n != "0" {
			t.Fatalf("media_blobs rows after document 5xx: %s", n)
		}
		if extra := addedKeys(before, s.objectKeys(t)); len(extra) != 0 {
			t.Fatalf("orphan objects after document 5xx: %v", extra)
		}

		s.proxy.setMode(s3Pass)
		if _, err = uploadDocumentResult(ctx, c, payload); err != nil {
			t.Fatalf("document retry: %v", err)
		}
		if n := s.scalar(t, "SELECT COUNT(*) FROM media_blobs WHERE sha256 = ?", sha); n != "1" {
			t.Fatalf("media_blobs rows after document retry: %s", n)
		}
	})

	t.Run("kill dfs mid-upload", func(t *testing.T) {
		payload := solidJPEG(t, 9)
		sum := sha256.Sum256(payload)
		sha := hex.EncodeToString(sum[:])
		before := s.objectKeys(t)
		s.proxy.resetPuts()
		s.proxy.setMode(s3Hold)
		defer s.proxy.setMode(s3Pass)

		done := make(chan error, 1)
		go func() {
			done <- uploadJPEGErr(ctx, c, payload)
		}()
		waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
		defer cancelWait()
		if err := s.proxy.waitPut(waitCtx); err != nil {
			t.Fatalf("upload did not reach object storage: %v", err)
		}
		s.restartService(t, "dfs")
		s.proxy.setMode(s3Pass)
		select {
		case err := <-done:
			rpc, ok := asRPCError(err)
			if !ok || rpc.Code < 400 {
				t.Fatalf("killed dfs returned %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("upload did not return after dfs restart")
		}
		// The blob row is claimed before the put. A kill in that window
		// cannot drop the row. The object must still be absent, and the
		// retry must write it.
		if extra := addedKeys(before, s.objectKeys(t)); len(extra) != 0 {
			t.Fatalf("orphan objects after killed upload: %v", extra)
		}

		var photo *mtproto.Photo
		var last error
		for i := 0; i < 5; i++ {
			photo, last = uploadJPEGResult(ctx, c, payload)
			if last == nil {
				break
			}
			time.Sleep(time.Second)
		}
		if last != nil {
			t.Fatalf("retry after dfs restart: %v", last)
		}
		if n := s.scalar(t, "SELECT COUNT(*) FROM media_blobs WHERE sha256 = ?", sha); n != "1" {
			t.Fatalf("media_blobs rows after retry: %s", n)
		}
		downloadPhoto(t, ctx, c, photo)
	})
}

func uploadDocumentResult(ctx context.Context, c *Client, payload []byte) (*mtproto.Document, error) {
	fileID := randomID()
	if _, err := c.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       payload,
	}); err != nil {
		return nil, err
	}
	res, err := c.Invoke(ctx, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "reliability.bin",
			}).To_InputFile(),
			MimeType: "application/octet-stream",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
					FileName: "reliability.bin",
				}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
	})
	if err != nil {
		return nil, err
	}
	switch doc := res.(type) {
	case *mtproto.TLMessageMediaDocument:
		if doc.GetDocument() == nil || doc.GetDocument().GetId() == 0 {
			return nil, errUploadShape(res)
		}
		return doc.GetDocument(), nil
	default:
		return nil, errUploadShape(res)
	}
}

func uploadJPEGErr(ctx context.Context, c *Client, payload []byte) error {
	_, err := uploadJPEGResult(ctx, c, payload)
	return err
}

func uploadJPEGResult(ctx context.Context, c *Client, payload []byte) (*mtproto.Photo, error) {
	fileID := randomID()
	if _, err := c.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       payload,
	}); err != nil {
		return nil, err
	}
	res, err := c.Invoke(ctx, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "reliability.jpg",
			}).To_InputFile(),
		}).To_InputMedia(),
	})
	if err != nil {
		return nil, err
	}
	media, ok := res.(*mtproto.TLMessageMediaPhoto)
	if !ok || media.GetPhoto_FLAGPHOTO() == nil || media.GetPhoto_FLAGPHOTO().GetId() == 0 {
		return nil, errUploadShape(res)
	}
	return media.GetPhoto_FLAGPHOTO(), nil
}

type uploadShapeError struct{ obj mtproto.TLObject }

func errUploadShape(obj mtproto.TLObject) error { return uploadShapeError{obj} }

func (e uploadShapeError) Error() string { return "messages.uploadMedia returned an unexpected object" }

func downloadPhoto(t *testing.T, ctx context.Context, c *Client, photo *mtproto.Photo) {
	t.Helper()
	size := "x"
	var best int32
	for _, sz := range photo.GetSizes() {
		if sz.GetSize2() > best && sz.GetType() != "" {
			best = sz.GetSize2()
			size = sz.GetType()
		}
	}
	res, err := c.Invoke(ctx, &mtproto.TLUploadGetFile{
		Constructor: mtproto.CRC32_upload_getFile_be5335be,
		Location: mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
			Id:            photo.GetId(),
			AccessHash:    photo.GetAccessHash(),
			FileReference: photo.GetFileReference(),
			ThumbSize:     size,
		}).To_InputFileLocation(),
		Limit: 512 * 1024,
	})
	if err != nil {
		t.Fatalf("upload.getFile: %v", err)
	}
	if len(bytesOf(res)) == 0 {
		t.Fatal("upload.getFile returned no bytes")
	}
}

func addedKeys(before, after []string) []string {
	have := map[string]bool{}
	for _, key := range before {
		have[key] = true
	}
	var extra []string
	for _, key := range after {
		if !have[key] {
			extra = append(extra, key)
		}
	}
	return extra
}
