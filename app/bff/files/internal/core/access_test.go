package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/pkg/mediaaccess"
)

func TestGetFileRequiresStickerAccessHash(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	set, err := catalog.Must().GetSet(0, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: catalog.ShortTwemoji,
	}).To_InputStickerSet(), 0)
	if err != nil || len(set.GetDocuments()) == 0 {
		t.Fatal(err)
	}
	doc := set.GetDocuments()[0]
	c := fileCore()

	_, err = c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
			Id: doc.GetId(), AccessHash: 1,
		}).To_InputFileLocation(),
		Limit: 32,
	})
	if err != mtproto.ErrFileIdInvalid {
		t.Fatalf("bad sticker hash: %v", err)
	}
	_, err = c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{
		Location: mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
			Id: doc.GetId(), AccessHash: 1,
		}).To_InputFileLocation(),
	})
	if err != mtproto.ErrFileIdInvalid {
		t.Fatalf("bad sticker hashes: %v", err)
	}

	got, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
			Id: doc.GetId(), AccessHash: doc.GetAccessHash(),
		}).To_InputFileLocation(),
		Limit: 64,
	})
	if err != nil || len(got.GetBytes()) == 0 {
		t.Fatalf("sticker download %v bytes %d", err, len(got.GetBytes()))
	}
}

func TestGetFileMissingPhotoIsFileIDInvalid(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	c := fileCore()
	_, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
			Id: 99, AccessHash: 1, ThumbSize: "x",
		}).To_InputFileLocation(),
		Limit: 32,
	})
	if err != mtproto.ErrFileIdInvalid {
		t.Fatalf("missing photo %v", err)
	}
}

func TestGetFileSecureLocationRejected(t *testing.T) {
	c := fileCore()
	_, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputSecureFileLocation(&mtproto.InputFileLocation{
			Id: 4, AccessHash: 4,
		}).To_InputFileLocation(),
		Limit: 32,
	})
	if err != mtproto.ErrFileIdInvalid {
		t.Fatalf("secure %v", err)
	}
}

func TestGetFileLogModeStillReads(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	t.Setenv(mediaaccess.EnvEnforce, "log")
	downloads := 0
	downloadFile = func(context.Context, *mtproto.InputFileLocation, int64, int32) (*mtproto.Upload_File, error) {
		downloads++
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte("ok")}).To_Upload_File(), nil
	}
	authorizeDownload = func(c *FilesCore, location *mtproto.InputFileLocation) error {
		return c.authorizeLocation(location)
	}
	t.Cleanup(func() {
		downloadFile = nil
		authorizeDownload = nil
	})
	c := fileCore()
	got, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
			Id: 99, AccessHash: 1, ThumbSize: "x",
		}).To_InputFileLocation(),
		Limit: 32,
	})
	if err != nil || string(got.GetBytes()) != "ok" || downloads != 1 {
		t.Fatalf("log mode err %v downloads %d", err, downloads)
	}
}
