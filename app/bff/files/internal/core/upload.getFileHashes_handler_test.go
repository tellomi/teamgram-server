package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/cdn"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/media/retention"
)

func TestHashFilePartsMatchPartHashes(t *testing.T) {
	body := make([]byte, cdn.PartBytes+80)
	for i := range body {
		body[i] = byte(i)
	}
	read := func(off int64, n int32) ([]byte, error) {
		if off >= int64(len(body)) {
			return []byte{}, nil
		}
		end := off + int64(n)
		if end > int64(len(body)) {
			end = int64(len(body))
		}
		return append([]byte(nil), body[off:end]...), nil
	}
	for _, offset := range []int64{0, 1, int64(cdn.PartBytes) - 1, int64(cdn.PartBytes), int64(len(body)), int64(len(body)) + 10} {
		got, err := hashFileParts(offset, read)
		if err != nil {
			t.Fatal(err)
		}
		want, err := cdn.PartHashes(body, offset)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("offset %d: %d hashes, want %d", offset, len(got), len(want))
		}
		for i := range want {
			if got[i].GetOffset_INT64() != want[i].GetOffset_INT64() || got[i].GetLimit() != want[i].GetLimit() || !bytes.Equal(got[i].GetHash(), want[i].GetHash()) {
				t.Fatalf("offset %d part %d: %+v vs %+v", offset, i, got[i], want[i])
			}
		}
	}
	if _, err := hashFileParts(-5, read); err != mtproto.ErrOffsetInvalid {
		t.Fatal(err)
	}
}

func TestUploadGetFileHashes(t *testing.T) {
	body := make([]byte, cdn.PartBytes+3)
	for i := range body {
		body[i] = byte(i)
	}
	downloadFile = func(_ context.Context, _ *mtproto.InputFileLocation, offset int64, limit int32) (*mtproto.Upload_File, error) {
		if offset >= int64(len(body)) {
			return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte{}}).To_Upload_File(), nil
		}
		end := offset + int64(limit)
		if end > int64(len(body)) {
			end = int64(len(body))
		}
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: append([]byte(nil), body[offset:end]...)}).To_Upload_File(), nil
	}
	t.Cleanup(func() { downloadFile = nil })
	c := fileCore()
	loc := docLoc(42)

	got, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	want, err := cdn.PartHashes(body, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDatas()) != 2 || !bytes.Equal(got.Datas[0].GetHash(), want[0].GetHash()) || got.Datas[1].GetLimit() != 3 {
		t.Fatalf("%d hashes", len(got.GetDatas()))
	}
	sum := sha256.Sum256(body[:cdn.PartBytes])
	if !bytes.Equal(got.Datas[0].GetHash(), sum[:]) || got.Datas[0].GetOffset_INT64() != 0 {
		t.Fatal("first part")
	}

	from, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: loc, Offset_INT64: int64(cdn.PartBytes)})
	if err != nil || len(from.GetDatas()) != 1 || from.Datas[0].GetOffset_INT64() != int64(cdn.PartBytes) {
		t.Fatal(err, from)
	}
	mid, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: loc, Offset_INT32: 1})
	if err != nil || len(mid.GetDatas()) != 1 || mid.Datas[0].GetOffset_INT64() != int64(cdn.PartBytes) {
		t.Fatal(err, mid)
	}
	if _, err = c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: loc, Offset_INT64: -1}); err != mtproto.ErrOffsetInvalid {
		t.Fatal(err)
	}
	if _, err = c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{}); err != mtproto.ErrLocationInvalid {
		t.Fatal(err)
	}
	legacy := mtproto.MakeTLInputFileLocation(&mtproto.InputFileLocation{VolumeId: 1, LocalId: 2}).To_InputFileLocation()
	if _, err = c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: legacy}); err != mtproto.ErrInputRequestInvalid {
		t.Fatal(err)
	}
}

func TestUploadGetFileHashesExpired(t *testing.T) {
	downloads := 0
	downloadFile = func(context.Context, *mtproto.InputFileLocation, int64, int32) (*mtproto.Upload_File, error) {
		downloads++
		return mtproto.MakeTLUploadFile(&mtproto.Upload_File{Bytes: []byte("x")}).To_Upload_File(), nil
	}
	t.Cleanup(func() { downloadFile = nil })
	c := fileCore()
	c.svcCtx = &svc.ServiceContext{Retention: retention.NewChecker(expiredSet{7: true}, 0)}
	if _, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: docLoc(7)}); err != mtproto.ErrFileIdInvalid {
		t.Fatal(err)
	}
	thumb := docLoc(7)
	thumb.ThumbSize = "m"
	got, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: thumb})
	if err != nil || len(got.GetDatas()) != 1 || downloads != 1 {
		t.Fatalf("thumb err %v downloads %d", err, downloads)
	}
}
