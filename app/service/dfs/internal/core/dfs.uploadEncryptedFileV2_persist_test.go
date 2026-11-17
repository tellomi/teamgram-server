package core

import (
	"context"
	"crypto/md5"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
)

type captureInt64 struct{ dst *int64 }

func (c captureInt64) Match(v driver.Value) bool {
	n, ok := v.(int64)
	if !ok || n == 0 {
		return false
	}
	*c.dst = n
	return true
}

func TestUploadEncryptedFilePersistsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	c := newPartCore(t)
	const fileID int64 = 4242
	c.idHook = func() int64 { return fileID }
	c.svcCtx.Dao.Encrypted = dao.NewEncryptedFiles(db)

	body := bytesRepeat(128, 0xab)
	writePart(t, c, 51, 0, body, 0)
	sum := md5.Sum(body)
	checksum := hex.EncodeToString(sum[:])

	var storedHash int64
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT access_hash").WithArgs(fileID).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("(?s)INSERT INTO encrypted_files.*expired_at.*\\b0\\b").
		WithArgs(fileID, captureInt64{dst: &storedHash}, int32(1), int64(128), int32(77), checksum, "4242.dat").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	uploaded := mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
		Id: 51, Parts: 1, Md5Checksum: checksum, KeyFingerprint: 77,
	}).To_InputEncryptedFile()
	out, err := c.DfsUploadEncryptedFileV2(&dfs.TLDfsUploadEncryptedFileV2{
		Creator: 7,
		File:    uploaded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetId() != fileID || out.GetAccessHash() != storedHash || storedHash == 0 {
		t.Fatalf("id=%d access=%d stored=%d", out.GetId(), out.GetAccessHash(), storedHash)
	}
	if out.GetSize2_INT64() != 128 || out.GetSize2_INT32() != 128 || out.GetDcId() != 1 || out.GetKeyFingerprint() != 77 {
		t.Fatalf("size=%d/%d dc=%d kf=%d", out.GetSize2_INT64(), out.GetSize2_INT32(), out.GetDcId(), out.GetKeyFingerprint())
	}

	// Same encrypted file id again: no second insert, and the first hash stays
	// even when this call would have minted a different one.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT access_hash").WithArgs(fileID).WillReturnRows(sqlmock.NewRows(
		[]string{"access_hash", "dc_id", "file_size", "key_fingerprint", "md5_checksum", "file_path"},
	).AddRow(storedHash, int32(1), int64(128), int32(77), checksum, "4242.dat"))
	mock.ExpectCommit()

	againFile := mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
		Id: 51, Parts: 1, Md5Checksum: stringsToUpper(checksum), KeyFingerprint: 88,
	}).To_InputEncryptedFile()
	again, err := c.DfsUploadEncryptedFileV2(&dfs.TLDfsUploadEncryptedFileV2{
		Creator: 7,
		File:    againFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.GetId() != fileID || again.GetAccessHash() != storedHash || again.GetKeyFingerprint() != 77 || again.GetSize2_INT64() != 128 {
		t.Fatalf("retry id=%d access=%d kf=%d size=%d", again.GetId(), again.GetAccessHash(), again.GetKeyFingerprint(), again.GetSize2_INT64())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveEncryptedFileDuplicateKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := dao.NewEncryptedFiles(db)
	row := dao.EncryptedFileRow{
		ID: 9, AccessHash: 3, DCID: 1, Size: 4096, KeyFingerprint: 4, MD5: "abcd", Path: "9.dat",
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT access_hash").WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("(?s)INSERT INTO encrypted_files.*expired_at.*\\b0\\b").
		WithArgs(int64(9), int64(3), int32(1), int64(4096), int32(4), "abcd", "9.dat").
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	mock.ExpectQuery("SELECT access_hash").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows(
		[]string{"access_hash", "dc_id", "file_size", "key_fingerprint", "md5_checksum", "file_path"},
	).AddRow(int64(11), int32(1), int64(4096), int32(4), "abcd", "9.dat"))
	mock.ExpectCommit()

	saved, err := store.Save(context.Background(), row)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != 9 || saved.AccessHash != 11 || saved.Size != 4096 || saved.KeyFingerprint != 4 {
		t.Fatalf("saved %+v", saved)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func bytesRepeat(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func stringsToUpper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - ('a' - 'A')
		}
	}
	return string(out)
}
