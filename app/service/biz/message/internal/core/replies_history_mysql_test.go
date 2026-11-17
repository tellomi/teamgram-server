package core

// These tests call repliesdelete.DeleteHistory and MessageGetHistoryMessages.
// Their delete callback is UPDATE messages SET deleted=1 for the caller's
// box ids. They do not call MsgDeleteMessages or ContactsBlockFromReplies.
// The BFF request shape is TestBlockFromRepliesDeleteHistoryWalksPast500.
// MsgDeleteMessages setting deleted=1 is TestBlockFromRepliesDeletePersistsCallerRows.
// None of these is a socket-to-database e2e.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/repliesdelete"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/pkg/msgexpiry"
	"google.golang.org/grpc/status"

	_ "github.com/go-sql-driver/mysql"
)

const repliesHistorySchema = `
CREATE TABLE messages (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  user_message_box_id INT NOT NULL,
  dialog_id1 BIGINT NOT NULL,
  dialog_id2 BIGINT NOT NULL,
  dialog_message_id BIGINT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  random_id BIGINT NOT NULL DEFAULT 0,
  message_filter_type INT NOT NULL DEFAULT 0,
  message_data JSON NULL,
  message VARCHAR(6000) NOT NULL DEFAULT '',
  mentioned TINYINT NOT NULL DEFAULT 0,
  media_unread TINYINT NOT NULL DEFAULT 0,
  pinned TINYINT NOT NULL DEFAULT 0,
  has_reaction TINYINT NOT NULL DEFAULT 0,
  reaction VARCHAR(256) NOT NULL DEFAULT '',
  reaction_date BIGINT NOT NULL DEFAULT 0,
  reaction_unread TINYINT NOT NULL DEFAULT 0,
  saved_peer_type INT NOT NULL DEFAULT 0,
  saved_peer_id BIGINT NOT NULL DEFAULT 0,
  date2 BIGINT NOT NULL DEFAULT 0,
  ttl_period INT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  UNIQUE KEY box_id (user_id, user_message_box_id)
);`

func TestRepliesHistoryReadErrorIsNotAnEmptyPage(t *testing.T) {
	db, dsn := newRepliesSchema(t)
	msgCore := newRepliesMessageCore(t, dsn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(ctx, msgCore.svcCtx).MessageGetHistoryMessages(&message.TLMessageGetHistoryMessages{
		UserId:     1,
		PeerType:   mtproto.PEER_USER,
		PeerId:     1271266957,
		OffsetDate: repliesdelete.RawOffsetDate,
		Limit:      repliesdelete.Page,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("raw history error %v", err)
	}
	_, err = msgCore.svcCtx.Dao.GetOffsetIdBackwardHistoryMessages(ctx, 1, mtproto.MakePeerUtil(mtproto.PEER_USER, 1271266957), 1<<31-1, 0, 0, 100, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dao history error %v", err)
	}
	_ = db
}

func TestRepliesHistoryWalkMySQL(t *testing.T) {
	db, dsn := newRepliesSchema(t)
	const (
		caller int64 = 910442301
		other  int64 = 910442302
		author int64 = 910442315
		noise  int64 = 910442399
		keep   int32 = 10
	)
	repliesPeer := int64(1271266957)
	did := mtproto.MakeDialogId(caller, mtproto.PEER_USER, repliesPeer)
	otherDid := mtproto.MakeDialogId(other, mtproto.PEER_USER, repliesPeer)
	private := mtproto.MakeDialogId(caller, mtproto.PEER_USER, author)
	for id := int32(2000); id >= 1481; id-- {
		insertRepliesRow(t, db, caller, id, did, repliesPeer, noise, 0, 0, 0)
	}
	insertRepliesRow(t, db, caller, 400, did, repliesPeer, author, 0, 0, 0)
	insertRepliesRow(t, db, caller, 100, did, repliesPeer, author, 0, 0, 0)
	insertRepliesRow(t, db, caller, keep, did, repliesPeer, author, 0, 0, 0)
	insertRepliesRow(t, db, caller, 77, did, repliesPeer, author, 0, 0, 1)
	insertRepliesRow(t, db, caller, 9001, private, caller, author, 0, 0, 0)
	insertRepliesRow(t, db, other, keep, otherDid, repliesPeer, author, 0, 0, 0)

	msgCore := newRepliesMessageCore(t, dsn)
	live, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := repliesdelete.DeleteHistory(live, repliesLoad(msgCore), func(ctx context.Context, offset int32) ([]*mtproto.MessageBox, error) {
		calls++
		if calls == 2 {
			cancel()
			return repliesRead(msgCore, caller, repliesPeer)(live, offset)
		}
		return repliesRead(msgCore, caller, repliesPeer)(ctx, offset)
	}, func(ctx context.Context, ids []int32) error {
		return markDeleted(ctx, db, caller, ids)
	}, caller, keep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	if repliesDeleted(t, db, caller, keep) != 0 {
		t.Fatal("canceled walk deleted the trigger")
	}
	if _, err = repliesLoad(msgCore)(context.Background(), caller, keep); err != nil {
		t.Fatalf("trigger must stay readable: %v", err)
	}
	if err = repliesdelete.DeleteHistory(context.Background(), repliesLoad(msgCore), repliesRead(msgCore, caller, repliesPeer), func(ctx context.Context, ids []int32) error {
		return markDeleted(ctx, db, caller, ids)
	}, caller, keep); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int32{400, 100, keep} {
		if repliesDeleted(t, db, caller, id) != 1 {
			t.Fatalf("author %d not deleted", id)
		}
	}
	for _, id := range []int32{2000, 1481, 9001} {
		if repliesDeleted(t, db, caller, id) != 0 {
			t.Fatalf("caller row %d deleted", id)
		}
	}
	if repliesDeleted(t, db, caller, 77) != 1 || repliesDeleted(t, db, other, keep) != 0 {
		t.Fatal("deleted flag or other user changed")
	}
	_, err = repliesLoad(msgCore)(context.Background(), caller, keep)
	if status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatalf("spent trigger %v", err)
	}
}

func TestRepliesHistoryDeleteFailureRetries(t *testing.T) {
	db, dsn := newRepliesSchema(t)
	const (
		caller int64 = 910442401
		author int64 = 910442415
		keep   int32 = 10
	)
	repliesPeer := int64(1271266957)
	did := mtproto.MakeDialogId(caller, mtproto.PEER_USER, repliesPeer)
	for id := int32(700); id >= 101; id-- {
		insertRepliesRow(t, db, caller, id, did, repliesPeer, author, 0, 0, 0)
	}
	insertRepliesRow(t, db, caller, keep, did, repliesPeer, author, 0, 0, 0)
	msgCore := newRepliesMessageCore(t, dsn)
	calls := 0
	err := repliesdelete.DeleteHistory(context.Background(), repliesLoad(msgCore), repliesRead(msgCore, caller, repliesPeer), func(ctx context.Context, ids []int32) error {
		calls++
		if calls == 2 {
			return errors.New("delete failed")
		}
		return markDeleted(ctx, db, caller, ids)
	}, caller, keep)
	if err == nil || err.Error() != "delete failed" {
		t.Fatalf("delete error %v", err)
	}
	if repliesDeleted(t, db, caller, keep) != 0 {
		t.Fatal("failed walk deleted the trigger")
	}
	if _, err = repliesLoad(msgCore)(context.Background(), caller, keep); err != nil {
		t.Fatalf("trigger after failed delete: %v", err)
	}
	if err = repliesdelete.DeleteHistory(context.Background(), repliesLoad(msgCore), repliesRead(msgCore, caller, repliesPeer), func(ctx context.Context, ids []int32) error {
		return markDeleted(ctx, db, caller, ids)
	}, caller, keep); err != nil {
		t.Fatal(err)
	}
	if repliesDeleted(t, db, caller, 700) != 1 || repliesDeleted(t, db, caller, 101) != 1 || repliesDeleted(t, db, caller, keep) != 1 {
		t.Fatal("retry left an author row")
	}
}

func TestRepliesHistoryExpiryPageDoesNotEndTheWalk(t *testing.T) {
	t.Setenv(msgexpiry.EnvEnabled, "1")
	db, dsn := newRepliesSchema(t)
	const (
		caller int64 = 910442501
		author int64 = 910442515
		noise  int64 = 910442599
		keep   int32 = 10
	)
	repliesPeer := int64(1271266957)
	did := mtproto.MakeDialogId(caller, mtproto.PEER_USER, repliesPeer)
	for id := int32(200); id >= 101; id-- {
		insertRepliesRow(t, db, caller, id, did, repliesPeer, noise, 1000, 10, 0)
	}
	insertRepliesRow(t, db, caller, keep, did, repliesPeer, author, int32(time.Now().Unix()), 0, 0)
	msgCore := newRepliesMessageCore(t, dsn)
	filtered, err := msgCore.MessageGetHistoryMessages(&message.TLMessageGetHistoryMessages{
		UserId:   caller,
		PeerType: mtproto.PEER_USER,
		PeerId:   repliesPeer,
		Limit:    repliesdelete.Page,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.GetDatas()) != 0 {
		t.Fatalf("expiry filter returned %d", len(filtered.GetDatas()))
	}
	if err = repliesdelete.DeleteHistory(context.Background(), repliesLoad(msgCore), repliesRead(msgCore, caller, repliesPeer), func(ctx context.Context, ids []int32) error {
		return markDeleted(ctx, db, caller, ids)
	}, caller, keep); err != nil {
		t.Fatal(err)
	}
	if repliesDeleted(t, db, caller, keep) != 1 {
		t.Fatal("expired full page hid the older author row")
	}
	if repliesDeleted(t, db, caller, 200) != 0 {
		t.Fatal("expired other author was deleted")
	}
}

func repliesRead(msgCore *MessageCore, user, peer int64) repliesdelete.PageFunc {
	return func(ctx context.Context, offset int32) ([]*mtproto.MessageBox, error) {
		page, err := New(ctx, msgCore.svcCtx).MessageGetHistoryMessages(&message.TLMessageGetHistoryMessages{
			UserId:     user,
			PeerType:   mtproto.PEER_USER,
			PeerId:     peer,
			OffsetId:   offset,
			OffsetDate: repliesdelete.RawOffsetDate,
			Limit:      repliesdelete.Page,
		})
		if err != nil || page == nil {
			return nil, err
		}
		return page.GetDatas(), nil
	}
}

func repliesLoad(msgCore *MessageCore) repliesdelete.BoxLoad {
	return func(ctx context.Context, userID int64, id int32) (*mtproto.MessageBox, error) {
		return New(ctx, msgCore.svcCtx).MessageGetUserMessage(&message.TLMessageGetUserMessage{UserId: userID, Id: id})
	}
}

func newRepliesMessageCore(t *testing.T, dsn string) *MessageCore {
	t.Helper()
	sqlDB := sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
	return New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{Mysql: dao.NewMysql(sqlDB)}})
}

func newRepliesSchema(t *testing.T) (*sql.DB, string) {
	t.Helper()
	admin, dsn := openRepliesMySQL(t)
	schema := fmt.Sprintf("bfr_page_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`") })
	full := repliesSchemaDSN(dsn, schema)
	db, err := sql.Open("mysql", full)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec(repliesHistorySchema); err != nil {
		t.Fatal(err)
	}
	return db, full
}

func openRepliesMySQL(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	required := dsn != "" || os.Getenv("TEAMGRAM_REQUIRE_INBOX_MYSQL") == "1"
	if dsn == "" {
		if required {
			t.Fatal("TEAMGRAM_MYSQL_DSN is required")
		}
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		if required {
			t.Fatal(err)
		}
		t.Skipf("mysql not available: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err = admin.Ping(); err != nil {
		if required {
			t.Fatal(err)
		}
		t.Skipf("mysql not available: %v", err)
	}
	return admin, dsn
}

func repliesSchemaDSN(dsn, schema string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return dsn + "/" + schema
	}
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return dsn[:slash+1] + schema + rest[q:]
	}
	return dsn[:slash+1] + schema
}

func insertRepliesRow(t *testing.T, db *sql.DB, user int64, box int32, did mtproto.DialogID, peer, author int64, date, ttl, deleted int32) {
	t.Helper()
	raw := fmt.Sprintf(`{"predicate_name":"message","date":%d,"fwd_from":{"from_id":{"predicate_name":"peerUser","user_id":%d}}}`, date, author)
	_, err := db.Exec(`INSERT INTO messages (user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, message_data, message, date2, ttl_period, deleted)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		user, box, did.A, did.B, int64(box), int64(1271266957), mtproto.PEER_USER, peer, raw, "", date, ttl, deleted)
	if err != nil {
		t.Fatal(err)
	}
}

func markDeleted(ctx context.Context, db *sql.DB, user int64, ids []int32) error {
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `UPDATE messages SET deleted=1 WHERE user_id=? AND user_message_box_id=? AND deleted=0`, user, id); err != nil {
			return err
		}
	}
	return nil
}

func repliesDeleted(t *testing.T, db *sql.DB, user int64, box int32) int {
	t.Helper()
	var deleted int
	if err := db.QueryRow(`SELECT deleted FROM messages WHERE user_id=? AND user_message_box_id=?`, user, box).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	return deleted
}
