package core

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/cdn"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestGetFileStaysOnMasterWithoutCDN(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "")
	plain := []byte("master-bytes")
	var limits []int32
	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, _ int64, limit int32) (*mtproto.Upload_File, error) {
		limits = append(limits, limit)
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: plain}).To_Upload_File(), nil
	}
	t.Cleanup(func() { downloadFile = nil })
	c := fileCore()
	got, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		CdnSupported: true,
		Location:     docLoc(9),
		Limit:        32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_upload_file || !bytes.Equal(got.GetBytes(), plain) {
		t.Fatalf("%s %q", got.GetPredicateName(), got.GetBytes())
	}
	if len(limits) != 1 || limits[0] != 32 {
		t.Fatal(limits)
	}
}

func TestGetFileRedirectsWhenCDNConfigured(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "2|-----BEGIN PUBLIC KEY-----MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA")
	book := cdn.New()
	cdn.Install(book)
	t.Cleanup(func() {
		downloadFile = nil
		cdn.Install(cdn.New())
	})
	plain := []byte("hello-cdn-redirect-bytes")
	var calls int
	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, _ int64, limit int32) (*mtproto.Upload_File, error) {
		calls++
		if limit != cdn.MaxPlain+1 {
			t.Fatalf("probe limit %d", limit)
		}
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: plain}).To_Upload_File(), nil
	}
	c := fileCore()
	loc := docLoc(42)
	got, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		CdnSupported: true,
		Location:     loc,
		Limit:        4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_upload_fileCdnRedirect {
		t.Fatal(got.GetPredicateName())
	}
	if got.GetDcId() != 2 || len(got.GetFileToken()) == 0 || len(got.GetEncryptionKey()) != 32 || len(got.GetEncryptionIv()) != 16 || len(got.GetFileHashes()) == 0 {
		t.Fatalf("dc %d token %d key %d iv %d hashes %d", got.GetDcId(), len(got.GetFileToken()), len(got.GetEncryptionKey()), len(got.GetEncryptionIv()), len(got.GetFileHashes()))
	}
	again, err := c.UploadGetFile(&mtproto.TLUploadGetFile{CdnSupported: true, Location: loc, Limit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again.GetFileToken(), got.GetFileToken()) || calls != 1 {
		t.Fatalf("token reused calls=%d", calls)
	}
	off, err := c.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{FileToken: got.GetFileToken(), Limit: 4096})
	if err != nil || off.GetPredicateName() != mtproto.Predicate_upload_cdnFileReuploadNeeded {
		t.Fatal(err, off)
	}
	hashes, err := c.UploadReuploadCdnFile(&mtproto.TLUploadReuploadCdnFile{
		FileToken:    got.GetFileToken(),
		RequestToken: off.GetRequestToken(),
	})
	if err != nil || len(hashes.GetDatas()) != len(got.GetFileHashes()) {
		t.Fatal(err, hashes)
	}
	if !bytes.Equal(hashes.Datas[0].GetHash(), got.GetFileHashes()[0].GetHash()) {
		t.Fatal("hash mismatch")
	}
	chunk, err := c.UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{FileToken: got.GetFileToken(), Limit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decryptChunk(chunk.GetBytes(), got.GetEncryptionKey(), got.GetEncryptionIv(), 0), plain) {
		t.Fatal("plaintext")
	}

	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, _ int64, limit int32) (*mtproto.Upload_File, error) {
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte("no-redirect")}).To_Upload_File(), nil
	}
	plainFile, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		Location: docLoc(42),
		Limit:    16,
	})
	if err != nil || plainFile.GetPredicateName() != mtproto.Predicate_upload_file || string(plainFile.GetBytes()) != "no-redirect" {
		t.Fatal(err, plainFile)
	}
	secret, err := c.UploadGetFile(&mtproto.TLUploadGetFile{
		CdnSupported: true,
		Location:     mtproto.MakeTLInputEncryptedFileLocation(&mtproto.InputFileLocation{Id: 3, AccessHash: 4}).To_InputFileLocation(),
		Limit:        8,
	})
	if err != nil || secret.GetPredicateName() != mtproto.Predicate_upload_file {
		t.Fatal(err, secret)
	}
}

func TestGetFileSkipsRedirectForLargeFile(t *testing.T) {
	t.Setenv("TELLOMI_CDN_PUBLIC_KEYS", "2|-----BEGIN PUBLIC KEY-----MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA")
	cdn.Install(cdn.New())
	t.Cleanup(func() {
		downloadFile = nil
		cdn.Install(cdn.New())
	})
	var limits []int32
	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, _ int64, limit int32) (*mtproto.Upload_File, error) {
		limits = append(limits, limit)
		if limit == cdn.MaxPlain+1 {
			return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: make([]byte, cdn.MaxPlain+1)}).To_Upload_File(), nil
		}
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte("slice")}).To_Upload_File(), nil
	}
	got, err := fileCore().UploadGetFile(&mtproto.TLUploadGetFile{
		CdnSupported: true,
		Location:     docLoc(99),
		Limit:        64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_upload_file || string(got.GetBytes()) != "slice" {
		t.Fatalf("%s %q", got.GetPredicateName(), got.GetBytes())
	}
	if len(limits) != 2 || limits[0] != cdn.MaxPlain+1 || limits[1] != 64 {
		t.Fatal(limits)
	}
}

func fileCore() *FilesCore {
	return &FilesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
}

func docLoc(id int64) *mtproto.InputFileLocation {
	return mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
		Id:         id,
		AccessHash: id + 1,
	}).To_InputFileLocation()
}

func decryptChunk(chunk, key, iv []byte, offset int64) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	ctrIV := append([]byte(nil), iv...)
	binary.BigEndian.PutUint32(ctrIV[len(ctrIV)-4:], uint32(offset/16))
	out := make([]byte, len(chunk))
	cipher.NewCTR(block, ctrIV).XORKeyStream(out, chunk)
	return out
}
