package ttlmedia

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/mediapurge"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const stripDialog int64 = 42

func TestStripSetsExpiredAtBeforeHistory(t *testing.T) {
	raw := ttlDocumentJSON(t, 55, 3)
	next, removed, err := rewrite(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].Kind != mtproto.MediaKindDocument || removed[0].ID != 55 {
		t.Fatalf("removed %+v", removed)
	}
	if len(mediapurge.FromJSON(next)) != 0 {
		t.Fatalf("stripped history still names a file: %s", next)
	}
	boxes := []userBox{{10, 100}, {20, 200}}

	t.Run("timed document", func(t *testing.T) {
		db, mock, trace := openStripDB(t)
		bindPurge(t, mock, db)
		expectReads(mock, mtproto.MediaKindDocument, 55, raw, boxes)
		expectNoOtherDocumentHolder(mock, 55, boxes)
		expectExpireColumn(mock, "documents", 1)
		expectCommit(mock, "documents", mtproto.MediaKindDocument, 55, len(boxes), len(boxes), true)
		s := storeWithForget(db, trace)
		if err := s.Strip(context.Background(), stripDialog); err != nil {
			t.Fatal(err)
		}
		assertExpireBeforeStrip(t, *trace, mtproto.MediaKindDocument, 55)
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("view-once photo", func(t *testing.T) {
		photo := ttlPhotoJSON(t, 77, mtproto.ViewOnceTTL)
		db, mock, trace := openStripDB(t)
		s := storeWithForget(db, trace)
		s.holdOutside = func(context.Context, string, int64, []boxRow) (bool, error) { return false, nil }
		expectReads(mock, mtproto.MediaKindPhoto, 77, photo, boxes)
		expectExpireColumn(mock, "photos", 1)
		expectCommit(mock, "photos", mtproto.MediaKindPhoto, 77, len(boxes), len(boxes), true)
		if err := s.Strip(context.Background(), stripDialog); err != nil {
			t.Fatal(err)
		}
		assertExpireBeforeStrip(t, *trace, mtproto.MediaKindPhoto, 77)
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStripRetryFinishesExpiredAtAfterHistory(t *testing.T) {
	live := ttlDocumentJSON(t, 55, 3)
	stripped, _, err := rewrite(live)
	if err != nil {
		t.Fatal(err)
	}
	again, removed, err := rewrite(stripped)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("already-empty history still removed %+v", removed)
	}
	updates := 0
	if again != stripped {
		updates = 2
	}
	boxes := []userBox{{10, 100}, {20, 200}}
	db, mock, trace := openStripDB(t)
	s := storeWithForget(db, trace)
	s.holdOutside = func(context.Context, string, int64, []boxRow) (bool, error) { return false, nil }
	expectReads(mock, mtproto.MediaKindDocument, 55, stripped, boxes)
	expectExpireColumn(mock, "documents", 1)
	expectCommit(mock, "documents", mtproto.MediaKindDocument, 55, len(boxes), updates, true)
	if err := s.Strip(context.Background(), stripDialog); err != nil {
		t.Fatal(err)
	}
	assertExpireBeforeStrip(t, *trace, mtproto.MediaKindDocument, 55)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStripRollsBackWhenHistoryWriteFails(t *testing.T) {
	raw := ttlDocumentJSON(t, 55, 3)
	boxes := []userBox{{10, 100}, {20, 200}}
	db, mock, trace := openStripDB(t)
	s := storeWithForget(db, trace)
	s.holdOutside = func(context.Context, string, int64, []boxRow) (bool, error) { return false, nil }
	expectReads(mock, mtproto.MediaKindDocument, 55, raw, boxes)
	expectExpireColumn(mock, "documents", 1)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE documents SET expired_at").WithArgs(sqlmock.AnyArg(), int64(55)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SET message_data").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SET message_data").WillReturnError(errors.New("boom"))
	mock.ExpectRollback()
	err := s.Strip(context.Background(), stripDialog)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err %v", err)
	}
	assertExpireBeforeStrip(t, *trace, mtproto.MediaKindDocument, 55)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStripKeepsFileWhenAnotherMessageHoldsIt(t *testing.T) {
	raw := ttlDocumentJSON(t, 55, 3)
	boxes := []userBox{{10, 100}, {20, 200}}
	db, mock, trace := openStripDB(t)
	bindPurge(t, mock, db)
	expectReads(mock, mtproto.MediaKindDocument, 55, raw, boxes)
	mock.ExpectQuery("information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("messages"))
	args := []driver.Value{int64(55), int64(10), int32(100), int64(20), int32(200)}
	mock.ExpectQuery("media_document_id").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"pinned"}).AddRow(0))
	expectCommitWithoutExpire(mock, len(boxes))
	s := storeWithForget(db, trace)
	if err := s.Strip(context.Background(), stripDialog); err != nil {
		t.Fatal(err)
	}
	for _, line := range *trace {
		if strings.Contains(line, "SET expired_at") || strings.HasPrefix(line, "forget:") {
			t.Fatalf("expired a file another message still holds: %s", line)
		}
	}
	if !traceHas(*trace, "SET message_data") {
		t.Fatalf("history was not stripped: %s", strings.Join(*trace, "\n"))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStripHoldErrorDoesNotRewriteHistory(t *testing.T) {
	raw := ttlDocumentJSON(t, 55, 3)
	boxes := []userBox{{10, 100}}
	db, mock, _ := openStripDB(t)
	s := &Store{db: db, holdOutside: func(context.Context, string, int64, []boxRow) (bool, error) {
		return false, errors.New("hold failed")
	}}
	expectReads(mock, mtproto.MediaKindDocument, 55, raw, boxes)
	err := s.Strip(context.Background(), stripDialog)
	if err == nil || !strings.Contains(err.Error(), "hold failed") {
		t.Fatalf("err %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStripMissedShardDoesNotExpire(t *testing.T) {
	db, mock, trace := openStripDB(t)
	mock.ExpectQuery("FROM message_ttl_open").WillReturnRows(
		sqlmock.NewRows([]string{"media_kind", "media_id"}).AddRow(mtproto.MediaKindDocument, int64(55)))
	mock.ExpectQuery("information_schema.tables").WillReturnRows(
		sqlmock.NewRows([]string{"table_name"}).AddRow("messages"))
	mock.ExpectQuery("dialog_message_id").WillReturnRows(
		sqlmock.NewRows([]string{"user_id", "user_message_box_id", "message_data"}))
	mock.ExpectExec("SET stripped").WillReturnResult(sqlmock.NewResult(0, 1))
	s := storeWithForget(db, trace)
	if err := s.Strip(context.Background(), stripDialog); err != nil {
		t.Fatal(err)
	}
	if traceHas(*trace, "SET expired_at") || traceHas(*trace, "forget:") {
		t.Fatalf("missed shard expired the file: %s", strings.Join(*trace, "\n"))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type userBox struct {
	user int64
	box  int32
}

func openStripDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	trace := []string{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		trace = append(trace, actual)
		if strings.Contains(actual, expected) {
			return nil
		}
		return fmt.Errorf("SQL %q does not contain %q", actual, expected)
	})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock, &trace
}

func storeWithForget(db *sql.DB, trace *[]string) *Store {
	return &Store{db: db, forget: func(kind string, id int64) {
		*trace = append(*trace, "forget:"+kind+":"+strconv.FormatInt(id, 10))
	}}
}

func bindPurge(t *testing.T, mock sqlmock.Sqlmock, db *sql.DB) {
	t.Helper()
	for _, name := range []string{"message_media_refs", "media_purge_queue", "media_external_refs"} {
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS " + name).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mediapurge.Bind(db)
	t.Cleanup(func() { mediapurge.Bind(nil) })
}

func expectReads(mock sqlmock.Sqlmock, kind string, id int64, raw string, boxes []userBox) {
	mock.ExpectQuery("FROM message_ttl_open").WillReturnRows(
		sqlmock.NewRows([]string{"media_kind", "media_id"}).AddRow(kind, id))
	mock.ExpectQuery("information_schema.tables").WillReturnRows(
		sqlmock.NewRows([]string{"table_name"}).AddRow("messages"))
	rows := sqlmock.NewRows([]string{"user_id", "user_message_box_id", "message_data"})
	for _, b := range boxes {
		rows.AddRow(b.user, b.box, raw)
	}
	mock.ExpectQuery("dialog_message_id").WillReturnRows(rows)
}

func expectNoOtherDocumentHolder(mock sqlmock.Sqlmock, id int64, boxes []userBox) {
	mock.ExpectQuery("information_schema.tables").WillReturnRows(
		sqlmock.NewRows([]string{"table_name"}).AddRow("messages"))
	args := make([]driver.Value, 0, 1+len(boxes)*2)
	args = append(args, id)
	for _, b := range boxes {
		args = append(args, b.user, b.box)
	}
	mock.ExpectQuery("media_document_id").WithArgs(args...).WillReturnError(sql.ErrNoRows)
	refs := sqlmock.NewRows([]string{"box_kind", "owner_id", "box_id"})
	for _, b := range boxes {
		refs.AddRow(mediapurge.BoxUser, b.user, b.box)
	}
	mock.ExpectQuery("box_kind, owner_id, box_id").WillReturnRows(refs)
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

func expectExpireColumn(mock sqlmock.Sqlmock, table string, n int) {
	mock.ExpectQuery("COLUMN_NAME = 'expired_at'").WithArgs(table).WillReturnRows(
		sqlmock.NewRows([]string{"c"}).AddRow(n))
}

func expectCommit(mock sqlmock.Sqlmock, table, kind string, id int64, nBoxes, nUpdates int, enqueue bool) {
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE "+table+" SET expired_at").WithArgs(sqlmock.AnyArg(), id).WillReturnResult(sqlmock.NewResult(0, 1))
	for i := 0; i < nUpdates; i++ {
		mock.ExpectExec("SET message_data").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	for i := 0; i < nBoxes; i++ {
		mock.ExpectQuery("media_kind, media_id FROM message_media_refs").WillReturnRows(
			sqlmock.NewRows([]string{"media_kind", "media_id"}).AddRow(kind, id))
		mock.ExpectExec("DELETE FROM message_media_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	if enqueue {
		mock.ExpectExec("INSERT INTO media_purge_queue").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("SET stripped").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectCommitWithoutExpire(mock sqlmock.Sqlmock, nBoxes int) {
	mock.ExpectBegin()
	for i := 0; i < nBoxes; i++ {
		mock.ExpectExec("SET message_data").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	for i := 0; i < nBoxes; i++ {
		mock.ExpectQuery("media_kind, media_id FROM message_media_refs").WillReturnRows(
			sqlmock.NewRows([]string{"media_kind", "media_id"}))
		mock.ExpectExec("DELETE FROM message_media_refs").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("SET stripped").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func assertExpireBeforeStrip(t *testing.T, trace []string, kind string, id int64) {
	t.Helper()
	forgetAt, expireAt, stripAt := -1, -1, -1
	token := "forget:" + kind + ":" + strconv.FormatInt(id, 10)
	for i, line := range trace {
		switch {
		case line == token && forgetAt < 0:
			forgetAt = i
		case strings.Contains(line, "SET expired_at") && expireAt < 0:
			expireAt = i
		case strings.Contains(line, "SET message_data") && stripAt < 0:
			stripAt = i
		}
	}
	if expireAt < 0 {
		t.Fatalf("expired_at was not set\n%s", strings.Join(trace, "\n"))
	}
	if forgetAt < 0 || forgetAt > expireAt {
		t.Fatalf("getFile cache drop at %d, expired_at at %d\n%s", forgetAt, expireAt, strings.Join(trace, "\n"))
	}
	if stripAt >= 0 && expireAt > stripAt {
		t.Fatalf("history rewrite at %d before expired_at at %d\n%s", stripAt, expireAt, strings.Join(trace, "\n"))
	}
}

func traceHas(trace []string, sub string) bool {
	for _, line := range trace {
		if strings.Contains(line, sub) {
			return true
		}
	}
	return false
}

func ttlDocumentJSON(t *testing.T, id int64, ttl int32) string {
	t.Helper()
	msg := &mtproto.Message{
		MediaUnread: true,
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			TtlSeconds: &wrapperspb.Int32Value{Value: ttl},
			Document:   mtproto.MakeTLDocument(&mtproto.Document{Id: id}).To_Document(),
		}).To_MessageMedia(),
	}
	return mustJSON(t, msg)
}

func ttlPhotoJSON(t *testing.T, id int64, ttl int32) string {
	t.Helper()
	msg := &mtproto.Message{
		MediaUnread: true,
		Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			TtlSeconds:      &wrapperspb.Int32Value{Value: ttl},
			Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: id}).To_Photo(),
		}).To_MessageMedia(),
	}
	return mustJSON(t, msg)
}

func mustJSON(t *testing.T, msg *mtproto.Message) string {
	t.Helper()
	buf, err := jsonx.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf)
}
