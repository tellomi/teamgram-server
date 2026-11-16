package mysql_dao

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"

	_ "github.com/go-sql-driver/mysql"
)

// TestPrivateReplyLookupIncludesDeleted is the private-chat half of a reply
// to a message both sides already revoked. SelectPeerUserMessage still
// requires deleted = 0, which is what a basic group keeps using. The
// including-deleted lookup is only for PEER_USER.
func TestPrivateReplyLookupIncludesDeleted(t *testing.T) {
	db := openReplyLookupMySQL(t)
	ctx := context.Background()
	const schema = "teamgram_reply_del_910448"
	if _, err := db.Exec(ctx, "DROP DATABASE IF EXISTS "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "CREATE DATABASE "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "DROP DATABASE IF EXISTS "+schema)
	})
	dsn := replyLookupDSN()
	sep := "?"
	if len(dsn) > 0 && containsQ(dsn) {
		sep = "&"
	}
	scoped, err := sqlx.Open(&sqlx.Config{DSN: dsn + schema + sep + "charset=utf8mb4&parseTime=true"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = scoped.Exec(ctx, `CREATE TABLE messages (
		id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		user_message_box_id INT NOT NULL,
		dialog_id1 BIGINT NOT NULL DEFAULT 0,
		dialog_id2 BIGINT NOT NULL DEFAULT 0,
		dialog_message_id BIGINT NOT NULL DEFAULT 0,
		sender_user_id BIGINT NOT NULL,
		peer_type INT NOT NULL DEFAULT 0,
		peer_id BIGINT NOT NULL DEFAULT 0,
		random_id BIGINT NOT NULL DEFAULT 0,
		message_filter_type INT NOT NULL DEFAULT 0,
		message_data JSON NULL,
		message VARCHAR(255) NOT NULL DEFAULT '',
		mentioned TINYINT NOT NULL DEFAULT 0,
		media_unread TINYINT NOT NULL DEFAULT 0,
		pinned TINYINT NOT NULL DEFAULT 0,
		has_reaction TINYINT NOT NULL DEFAULT 0,
		reaction VARCHAR(32) NOT NULL DEFAULT '',
		reaction_date BIGINT NOT NULL DEFAULT 0,
		reaction_unread TINYINT NOT NULL DEFAULT 0,
		saved_peer_type INT NOT NULL DEFAULT 0,
		saved_peer_id BIGINT NOT NULL DEFAULT 0,
		date2 BIGINT NOT NULL DEFAULT 0,
		ttl_period INT NOT NULL DEFAULT 0,
		deleted TINYINT NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatal(err)
	}
	dao := NewMessagesDAO(scoped, 0)
	const (
		alice int64 = 910448001
		bob   int64 = 910448002
	)
	insert := `INSERT INTO messages (user_id, user_message_box_id, dialog_message_id, sender_user_id, deleted, message, message_data) VALUES (?,?,?,?,?,?,'{}')`
	for _, row := range []struct {
		user, dialog int64
		box          int32
		deleted      int
		text         string
	}{
		{alice, 77, 11, 1, "gone-256"},
		{bob, 77, 21, 1, "gone-256"},
		{alice, 78, 12, 0, "still-here"},
		{bob, 78, 22, 0, "still-here"},
	} {
		if _, err = scoped.Exec(ctx, insert, row.user, row.box, row.dialog, alice, row.deleted, row.text); err != nil {
			t.Fatal(err)
		}
	}
	gone, err := dao.SelectPeerUserMessage(ctx, bob, alice, 11)
	if err != nil {
		t.Fatal(err)
	}
	if gone != nil {
		t.Fatalf("deleted=0 lookup returned box %d", gone.UserMessageBoxId)
	}
	mapped, err := dao.SelectPeerUserMessageIncludingDeleted(ctx, bob, alice, 11)
	if err != nil {
		t.Fatal(err)
	}
	if mapped == nil || mapped.UserMessageBoxId != 21 {
		t.Fatalf("revoked private reply mapped to %#v", mapped)
	}
	liveOld, err := dao.SelectPeerUserMessage(ctx, bob, alice, 12)
	if err != nil {
		t.Fatal(err)
	}
	liveNew, err := dao.SelectPeerUserMessageIncludingDeleted(ctx, bob, alice, 12)
	if err != nil {
		t.Fatal(err)
	}
	if liveOld == nil || liveNew == nil || liveOld.UserMessageBoxId != 22 || liveNew.UserMessageBoxId != 22 {
		t.Fatalf("live row old=%v new=%v", liveOld, liveNew)
	}
}

func replyLookupDSN() string {
	if dsn := os.Getenv("TEAMGRAM_MYSQL_DSN"); dsn != "" {
		return trimDB(dsn)
	}
	return "root:@tcp(127.0.0.1:3306)/"
}

func openReplyLookupMySQL(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open(&sqlx.Config{DSN: replyLookupDSN() + "?charset=utf8mb4&parseTime=true"})
	if err != nil {
		replyLookupUnavailable(t, err)
	}
	if _, err = db.Exec(context.Background(), "SELECT 1"); err != nil {
		replyLookupUnavailable(t, err)
	}
	return db
}

func replyLookupUnavailable(t *testing.T, err error) {
	t.Helper()
	if os.Getenv("TEAMGRAM_REQUIRE_INBOX_MYSQL") == "1" {
		t.Fatalf("required loopback mysql is not available: %v", err)
	}
	t.Skipf("mysql not available: %v", err)
}

func trimDB(dsn string) string {
	// Keep the driver prefix and drop a database name so the test can
	// create its own schema. A DSN is user:pass@tcp(host:port)/db?params.
	at := 0
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == '/' && i+1 < len(dsn) && dsn[i-1] == ')' {
			at = i + 1
			break
		}
	}
	if at == 0 {
		return dsn
	}
	return dsn[:at]
}

func containsQ(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return true
		}
	}
	return false
}
