package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"google.golang.org/grpc/status"
)

func TestValidateEncryptedSource(t *testing.T) {
	info := &model.DfsFileInfo{FileTotalParts: 2, FirstFilePartSize: 8, FilePartSize: 8, LastFilePartSize: 8}
	uploaded := mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
		Id: 5, Parts: 2, Md5Checksum: "0123456789abcdef0123456789abcdef", KeyFingerprint: 9,
	}).To_InputEncryptedFile()
	if err := validateEncryptedSource(uploaded, info); err != nil {
		t.Fatal(err)
	}
	if err := encryptedChecksum(uploaded, "0123456789ABCDEF0123456789ABCDEF"); err != nil {
		t.Fatal(err)
	}
	if status.Convert(encryptedChecksum(uploaded, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")).Message() != "MD5_CHECKSUM_INVALID" {
		t.Fatal(encryptedChecksum(uploaded, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	}

	short := mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
		Id: 5, Parts: 2, Md5Checksum: "abcd",
	}).To_InputEncryptedFile()
	if status.Convert(encryptedChecksum(short, "abcd")).Message() != "MD5_CHECKSUM_INVALID" {
		t.Fatal("short md5")
	}

	missing := mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{Id: 5, Parts: 1}).To_InputEncryptedFile()
	if status.Convert(validateEncryptedSource(missing, &model.DfsFileInfo{FileTotalParts: 1})).Message() != "FILE_PART_0_MISSING" {
		t.Fatal(validateEncryptedSource(missing, &model.DfsFileInfo{FileTotalParts: 1}))
	}
	mismatch := mtproto.MakeTLInputEncryptedFileBigUploaded(&mtproto.InputEncryptedFile{Id: 5, Parts: 3, KeyFingerprint: 1}).To_InputEncryptedFile()
	if err := validateEncryptedSource(mismatch, info); err != mtproto.ErrFilePartsInvalid {
		t.Fatal(err)
	}
	if err := validateEncryptedSource(nil, info); err != mtproto.ErrFileEmtpy {
		t.Fatal(err)
	}
	ref := mtproto.MakeTLInputEncryptedFile(&mtproto.InputEncryptedFile{Id: 5, AccessHash: 1}).To_InputEncryptedFile()
	if err := validateEncryptedSource(ref, info); err != mtproto.ErrFileIdInvalid {
		t.Fatal(err)
	}
	if err := encryptedChecksum(mismatch, ""); err != nil {
		t.Fatal(err)
	}
	badParts := mtproto.MakeTLInputEncryptedFileBigUploaded(&mtproto.InputEncryptedFile{Id: 5, Parts: 0}).To_InputEncryptedFile()
	if err := validateEncryptedSource(badParts, info); err != mtproto.ErrFilePartsInvalid {
		t.Fatal(err)
	}
}
