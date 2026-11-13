package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// TestOrdinaryPhotoGetFileDoesNotOpenObjectStorage is the regression for
// ISSUE-S-MEDIA-LATENCY-JP-001. upload.getFile used to call objectstore.Open
// for every photo id. Open builds a new S3 client and warms connections, which
// measured about 7s before dfs even when the thumbnail was already local.
func TestOrdinaryPhotoGetFileDoesNotOpenObjectStorage(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	var probed atomic.Int32
	chatbox.SetWebPhotoReaderForTest(func(string) ([]byte, error) {
		probed.Add(1)
		time.Sleep(300 * time.Millisecond)
		return nil, mtproto.ErrFileIdInvalid
	})
	t.Cleanup(func() { chatbox.SetWebPhotoReaderForTest(nil) })

	// access_hash is not the photo id. That is an uploaded photo, not a
	// link preview (storeWebPhoto sets access_hash = id).
	loc := mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
		Id:         2121303527006408704,
		AccessHash: 99,
		ThumbSize:  "s",
	}).To_InputFileLocation()
	in := &mtproto.TLUploadGetFile{Location: loc, Limit: 64}

	// Authorize runs before the handler's own preview check. A miss must
	// fail without dialing.
	c := fileCore()
	start := time.Now()
	_, err := c.UploadGetFile(in)
	if probed.Load() != 0 || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("authorize probed %d times in %s", probed.Load(), time.Since(start))
	}
	if err == nil {
		t.Fatal("photo with no media row was served")
	}

	// The download itself also checks the preview cache before dfs. Stubbing
	// the download skips authorize, which is the path that used to dial.
	var downloads atomic.Int32
	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, _ int64, _ int32) (*mtproto.Upload_File, error) {
		downloads.Add(1)
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte("thumb")}).To_Upload_File(), nil
	}
	t.Cleanup(func() { downloadFile = nil })
	start = time.Now()
	got, err := c.UploadGetFile(in)
	elapsed := time.Since(start)
	if err != nil || string(got.GetBytes()) != "thumb" || downloads.Load() != 1 {
		t.Fatalf("dfs err %v downloads %d bytes %q", err, downloads.Load(), got.GetBytes())
	}
	if probed.Load() != 0 || elapsed > 200*time.Millisecond {
		t.Fatalf("handler probed %d times in %s", probed.Load(), elapsed)
	}
}

func TestLinkPreviewGetFileReadsPersistedPhotoOnce(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	body := []byte{0xff, 0xd8, 0xff, 0xd9}
	var probed atomic.Int32
	chatbox.SetWebPhotoReaderForTest(func(string) ([]byte, error) {
		probed.Add(1)
		return append([]byte(nil), body...), nil
	})
	t.Cleanup(func() { chatbox.SetWebPhotoReaderForTest(nil) })

	const id int64 = 88001122
	c := fileCore()
	loc := mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
		Id: id, AccessHash: id, ThumbSize: "m",
	}).To_InputFileLocation()
	in := &mtproto.TLUploadGetFile{Location: loc, Limit: 64}

	got, err := c.UploadGetFile(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.GetBytes()) != string(body) {
		t.Fatalf("bytes %q", got.GetBytes())
	}
	if _, err = c.UploadGetFile(in); err != nil {
		t.Fatal(err)
	}
	if probed.Load() != 1 {
		t.Fatalf("persisted photo reads = %d, want 1", probed.Load())
	}
}
