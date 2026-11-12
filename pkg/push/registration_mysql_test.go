package push

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

const transferDelete = "DELETE FROM devices WHERE token_type = ? AND token = ? AND BINARY token = BINARY ? AND user_id NOT IN (?,?)"
const sameUserDelete = "DELETE FROM devices WHERE user_id = ? AND token_type = ? AND token = ? AND BINARY token = BINARY ? AND auth_key_id <> ?"

func fixtureRegistration() Device {
	return Device{AuthKeyID: 90, UserID: 9, TokenType: TokenFCM, Token: "fixture-token", OtherUIDs: []int64{8, 8, 9, 0, -1}}
}

func expectRegistrationWrites(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(transferDelete)).WithArgs(int32(TokenFCM), "fixture-token", "fixture-token", int64(9), int64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(sameUserDelete)).WithArgs(int64(9), int32(TokenFCM), "fixture-token", "fixture-token", int64(90)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO devices").WithArgs(int64(90), int64(9), int32(TokenFCM), "fixture-token", 0, 0, "", "8,8,9,0,-1", "").WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestRegistrationMySQLAtomicTransfer(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectRegistrationWrites(mock)
	mock.ExpectCommit()
	if err := (&MySQL{db: db}).Upsert(context.Background(), fixtureRegistration()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationMySQLRollsBackOnInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(transferDelete)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(sameUserDelete)).WillReturnResult(sqlmock.NewResult(0, 1))
	want := errors.New("fixture insert failed")
	mock.ExpectExec("INSERT INTO devices").WillReturnError(want)
	mock.ExpectRollback()
	if err := (&MySQL{db: db}).Upsert(context.Background(), fixtureRegistration()); !errors.Is(err, want) {
		t.Fatalf("wrong rollback error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationMySQLRetriesWholeTransactionAfterDeadlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(transferDelete)).WillReturnError(&mysqlDriver.MySQLError{Number: 1213, Message: "fixture deadlock"})
	mock.ExpectRollback()
	mock.ExpectBegin()
	expectRegistrationWrites(mock)
	mock.ExpectCommit()
	if err := (&MySQL{db: db}).Upsert(context.Background(), fixtureRegistration()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
