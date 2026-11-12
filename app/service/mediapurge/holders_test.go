package mediapurge

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHeldOutsideIgnoresBoxesBeingStripped(t *testing.T) {
	db, mock := holdDB(t)
	mock.ExpectQuery("information_schema.tables").WillReturnRows(
		sqlmock.NewRows([]string{"table_name"}).AddRow("messages"))
	mock.ExpectQuery("media_document_id").WithArgs(int64(55), int64(10), int32(100), int64(20), int32(200)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("box_kind, owner_id, box_id").WillReturnRows(
		sqlmock.NewRows([]string{"box_kind", "owner_id", "box_id"}).
			AddRow(BoxUser, int64(10), int32(100)).
			AddRow(BoxUser, int64(20), int32(200)))
	expectDocumentPointMisses(mock)
	held, err := New(db).HeldOutside(context.Background(), Media{Kind: KindDocument, ID: 55}, []ExceptBox{{10, 100}, {20, 200}})
	if err != nil || held {
		t.Fatalf("held %v err %v", held, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHeldOutsideKeepsAnotherMessage(t *testing.T) {
	db, mock := holdDB(t)
	mock.ExpectQuery("information_schema.tables").WillReturnRows(
		sqlmock.NewRows([]string{"table_name"}).AddRow("messages"))
	mock.ExpectQuery("media_document_id").WithArgs(int64(55), int64(10), int32(100)).
		WillReturnRows(sqlmock.NewRows([]string{"pinned"}).AddRow(0))
	held, err := New(db).HeldOutside(context.Background(), Media{Kind: KindDocument, ID: 55}, []ExceptBox{{10, 100}})
	if err != nil || !held {
		t.Fatalf("held %v err %v", held, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func holdDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if strings.Contains(actual, expected) {
			return nil
		}
		return fmt.Errorf("SQL %q does not contain %q", actual, expected)
	})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func expectDocumentPointMisses(mock sqlmock.Sqlmock) {
	for _, q := range []string{
		"tellomi_sticker_items",
		"tellomi_sticker_recent",
		"tellomi_sticker_faved",
		"tellomi_saved_gifs",
		"user_saved_music",
		"emoji_status_document_id",
		"WHERE saved_music_id=?",
		"media_external_refs",
	} {
		mock.ExpectQuery(q).WillReturnError(sql.ErrNoRows)
	}
}
