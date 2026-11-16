package core

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/mediaaccess"
	"google.golang.org/grpc/status"
)

func TestAuthorizeEncrypted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c := fileCore()
	c.svcCtx = &svc.ServiceContext{Access: mediaaccess.New(db)}
	loc := func(id, hash int64) *mtproto.InputFileLocation {
		return mtproto.MakeTLInputEncryptedFileLocation(&mtproto.InputFileLocation{
			Id: id, AccessHash: hash,
		}).To_InputFileLocation()
	}

	mock.ExpectQuery("SELECT access_hash FROM encrypted_files").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"access_hash"}).AddRow(int64(42)))
	if err = c.authorizeEncrypted(loc(9, 42)); err != nil {
		t.Fatalf("matching hash: %v", err)
	}

	mock.ExpectQuery("SELECT access_hash FROM encrypted_files").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"access_hash"}).AddRow(int64(42)))
	if err = c.authorizeEncrypted(loc(9, 7)); status.Convert(err).Message() != "FILE_ID_INVALID" {
		t.Fatalf("wrong hash: %v", err)
	}

	mock.ExpectQuery("SELECT access_hash FROM encrypted_files").WithArgs(int64(8)).
		WillReturnError(sql.ErrNoRows)
	if err = c.authorizeEncrypted(loc(8, 42)); status.Convert(err).Message() != "FILE_ID_INVALID" {
		t.Fatalf("unknown id: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
