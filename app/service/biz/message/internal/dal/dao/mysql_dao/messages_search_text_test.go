package mysql_dao

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLikeSubstringEscapesAndGlobalSQLIsCaseInsensitive(t *testing.T) {
	if got := likeSubstring(`Hello`); got != `%Hello%` {
		t.Fatalf("plain %s", got)
	}
	if got := likeSubstring(`100%_a\b`); got != `%100\%\_a\\b%` {
		t.Fatalf("escaped %s", got)
	}
	sql := searchGlobalTextSelect + searchGlobalTextCursor + searchGlobalTextOrder
	if !strings.Contains(sql, "utf8mb4_unicode_ci") {
		t.Fatal("search must ignore case")
	}
	if !strings.Contains(sql, "message = ''") || !strings.Contains(sql, "message_data") {
		t.Fatal("empty plaintext column must fall back to message_data")
	}
	if strings.Contains(searchGlobalTextSelect, "sender_user_id =") {
		t.Fatal("search must include inbox and outbox")
	}
	if !strings.Contains(sql, "date2 < ?") || !strings.Contains(sql, "peer_type > ?") || !strings.Contains(sql, "peer_id > ?") {
		t.Fatal("cursor must be the date, peer, id tuple")
	}
	if !strings.Contains(sql, "peer_type = ? and peer_id = ? and user_message_box_id < ?") {
		t.Fatal("box id is only the tie-break on the cursor peer")
	}
	if strings.Contains(searchGlobalTextSelect, "user_message_box_id <") {
		t.Fatal("the first page must not bound box ids")
	}
	if !strings.Contains(searchGlobalTextOrder, "order by date2 desc, peer_type asc, peer_id asc, user_message_box_id desc") {
		t.Fatal(searchGlobalTextOrder)
	}
}

func TestEqualDateTupleQueryIsExecuted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if strings.Contains(actual, expected) {
			return nil
		}
		return fmt.Errorf("SQL %s does not contain %s", actual, expected)
	})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	query := strings.Replace(searchGlobalTextSelect, "__TABLE__", "messages", 1) + searchGlobalTextCursor + searchGlobalTextOrder
	// Page 2 after a user message on the same date: date 50, peer type 2, peer 7, box 9.
	mock.ExpectQuery("date2 < ?").WithArgs(
		int64(4), "%Hello%", "%Hello%",
		int32(50),
		int32(50), int32(2),
		int32(50), int32(2), int64(7),
		int32(50), int32(2), int64(7), int32(9),
		int32(10),
	).WillReturnRows(sqlmock.NewRows([]string{"user_message_box_id"}).AddRow(int32(8)))
	rows, err := db.QueryContext(context.Background(), query,
		int64(4), "%Hello%", "%Hello%",
		int32(50),
		int32(50), int32(2),
		int32(50), int32(2), int64(7),
		int32(50), int32(2), int64(7), int32(9),
		int32(10),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var id int32
	if err := rows.Scan(&id); err != nil || id != 8 {
		t.Fatalf("id %d %v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
