package core

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bwmarrin/snowflake"
	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	msgdao "github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	dialoghelper "github.com/teamgram/teamgram-server/app/service/biz/dialog"
	idgenhelper "github.com/teamgram/teamgram-server/app/service/idgen"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// TestPrivateUnpinAllPersistsBothSides calls messages.unpinAll's msg handler,
// which writes the sender and then calls the real inbox handler. MySQL holds
// the message rows, dialog rows, and user_pts_updates. Redis is the same
// miniredis the idgen sequence and the dialog cache use. Kafka is not part of
// this process: the sync client records the update the handler built.
func TestPrivateUnpinAllPersistsBothSides(t *testing.T) {
	admin, dsn := openPrivateAdmin(t)
	schema := fmt.Sprintf("unpin_priv_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`") })
	dbDSN := withSchema(dsn, schema)
	db, err := sql.Open("mysql", dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(privateUnpinSchema); err != nil {
		t.Fatal(err)
	}

	const (
		userA = int64(1001)
		userB = int64(1002)
		userC = int64(1003)
	)
	ab := mtproto.MakeDialogId(userA, mtproto.PEER_USER, userB)
	ac := mtproto.MakeDialogId(userA, mtproto.PEER_USER, userC)
	insertMessage(t, db, userA, 1, ab, userA, userB, "hello-1", 1, 0)
	insertMessage(t, db, userA, 2, ab, userA, userB, "hello-2", 1, 0)
	insertMessage(t, db, userB, 11, ab, userA, userA, "hello-1", 1, 0)
	insertMessage(t, db, userB, 12, ab, userA, userA, "hello-2", 1, 0)
	insertMessage(t, db, userB, 99, ab, userA, userA, "deleted-pin", 1, 1)
	insertMessage(t, db, userA, 9, ac, userA, userC, "other-dialog", 1, 0)
	insertDialog(t, db, userA, userB, 2, 8, 4, 50)
	insertDialog(t, db, userB, userA, 12, 7, 3, 40)
	insertDialog(t, db, userA, userC, 9, 9, 1, 30)

	mr := miniredis.RunT(t)
	t.Cleanup(func() { time.Sleep(40 * time.Millisecond) })
	ctx := context.Background()
	sqlDB := sqlx.NewMySQL(&sqlx.Config{DSN: dbDSN})
	store := msgdao.NewMysqlDao(sqlDB, 0)
	node, err := snowflake.NewNode(1)
	if err != nil {
		t.Fatal(err)
	}
	seq := kv.NewStore(kv.KvConf{{
		RedisConf: redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType},
		Weight:    100,
	}})
	idCore := idgenhelper.NewCore(ctx, &idgenhelper.ServiceContext{Dao: &idgenhelper.Dao{Node: node, KV: seq}})
	dlg := dialoghelper.NewDao(dialoghelper.Config{
		Mysql: sqlx.Config{DSN: dbDSN},
		Cache: cache.CacheConf{{
			RedisConf: redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType},
			Weight:    100,
		}},
	})
	dlg.InvalidateDelay = 10 * time.Millisecond
	box := &localSync{}
	dao := &msgdao.Dao{
		Mysql:        store,
		IDGenClient2: idgen_client.NewIDGenClient2Direct(seqClient{core: idCore}),
		DialogClient: localDialog{svc: &dialoghelper.ServiceContext{Dao: dlg}},
		SyncClient:   box,
	}
	dao.InboxClient = localInbox{dao: dao}

	out, err := New(ctx, &svc.ServiceContext{Dao: dao}).MsgUnpinAllMessages(&msgpb.TLMsgUnpinAllMessages{
		UserId:    userA,
		AuthKeyId: 77,
		PeerType:  mtproto.PEER_USER,
		PeerId:    userB,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetPts() == 0 || out.GetPtsCount() != 2 {
		t.Fatalf("affected history pts=%d count=%d", out.GetPts(), out.GetPtsCount())
	}
	assertNoPins(t, store, ctx, userA, ab)
	assertNoPins(t, store, ctx, userB, ab)
	still, err := store.MessagesDAO.SelectPinnedList(ctx, userA, ac.A, ac.B)
	if err != nil || len(still) != 1 || still[0].UserMessageBoxId != 9 || still[0].Message != "other-dialog" {
		t.Fatalf("other dialog %#v %v", still, err)
	}
	var deletedPinned int
	if err = db.QueryRow(`SELECT pinned FROM messages WHERE user_id=? AND user_message_box_id=99`, userB).Scan(&deletedPinned); err != nil {
		t.Fatal(err)
	}
	if deletedPinned != 1 {
		t.Fatalf("deleted pin row changed to %d", deletedPinned)
	}
	assertDialog(t, db, userA, userB, 0, 8, 4)
	assertDialog(t, db, userB, userA, 0, 7, 3)
	assertDialog(t, db, userA, userC, 9, 9, 1)
	assertText(t, db, userA, 1, "hello-1")
	assertText(t, db, userB, 12, "hello-2")

	aRow := assertPtsRow(t, store, ctx, userA, out.GetPts(), out.GetPtsCount(), []int32{2, 1})
	if len(box.notMe) != 1 || box.notMe[0].UserId != userA || box.notMe[0].PermAuthKeyId != 77 {
		t.Fatalf("sender sync %#v", box.notMe)
	}
	assertSameUpdate(t, updatesOf(t, box.notMe[0].Updates), aRow, userB)
	if len(box.push) != 1 || box.push[0].UserId != userB {
		t.Fatalf("peer sync %#v", box.push)
	}
	bUpdate := updatesOf(t, box.push[0].Updates)
	bRow := assertPtsRow(t, store, ctx, userB, bUpdate.GetPts_INT32(), bUpdate.GetPtsCount(), []int32{12, 11})
	assertSameUpdate(t, bUpdate, bRow, userA)
}

const privateUnpinSchema = `
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
);
CREATE TABLE dialogs (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  peer_dialog_id BIGINT NOT NULL DEFAULT 0,
  pinned_msg_id INT NOT NULL DEFAULT 0,
  top_message INT NOT NULL DEFAULT 0,
  unread_count INT NOT NULL DEFAULT 0,
  date2 BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  UNIQUE KEY peer (user_id, peer_type, peer_id)
);
CREATE TABLE user_pts_updates (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  pts INT NOT NULL,
  pts_count INT NOT NULL,
  update_type TINYINT NOT NULL DEFAULT 0,
  update_data JSON NOT NULL,
  date2 BIGINT NOT NULL
);
`

func openPrivateAdmin(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	required := dsn != ""
	if dsn == "" {
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

func withSchema(dsn, schema string) string {
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

func insertMessage(t *testing.T, db *sql.DB, user int64, box int, did mtproto.DialogID, sender, peer int64, text string, pinned, deleted int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO messages (user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, message_data, message, pinned, deleted)
		VALUES (?,?,?,?,?,?,?,?, '{}', ?, ?, ?)`,
		user, box, did.A, did.B, int64(box), sender, mtproto.PEER_USER, peer, text, pinned, deleted)
	if err != nil {
		t.Fatal(err)
	}
}

func insertDialog(t *testing.T, db *sql.DB, user, peer int64, pinned, top, unread int, date int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO dialogs (user_id, peer_type, peer_id, pinned_msg_id, top_message, unread_count, date2) VALUES (?,?,?,?,?,?,?)`,
		user, mtproto.PEER_USER, peer, pinned, top, unread, date)
	if err != nil {
		t.Fatal(err)
	}
}

func assertNoPins(t *testing.T, store *msgdao.Mysql, ctx context.Context, user int64, did mtproto.DialogID) {
	t.Helper()
	list, err := store.MessagesDAO.SelectPinnedList(ctx, user, did.A, did.B)
	if err != nil || len(list) != 0 {
		t.Fatalf("user %d still pinned %#v %v", user, list, err)
	}
}

func assertDialog(t *testing.T, db *sql.DB, user, peer int64, pinned, top, unread int) {
	t.Helper()
	var gotPinned, gotTop, gotUnread int
	err := db.QueryRow(`SELECT pinned_msg_id, top_message, unread_count FROM dialogs WHERE user_id=? AND peer_type=? AND peer_id=?`,
		user, mtproto.PEER_USER, peer).Scan(&gotPinned, &gotTop, &gotUnread)
	if err != nil {
		t.Fatal(err)
	}
	if gotPinned != pinned || gotTop != top || gotUnread != unread {
		t.Fatalf("dialog %d->%d pinned=%d top=%d unread=%d", user, peer, gotPinned, gotTop, gotUnread)
	}
}

func assertText(t *testing.T, db *sql.DB, user int64, box int, text string) {
	t.Helper()
	var got string
	var pinned int
	if err := db.QueryRow(`SELECT message, pinned FROM messages WHERE user_id=? AND user_message_box_id=?`, user, box).Scan(&got, &pinned); err != nil {
		t.Fatal(err)
	}
	if got != text || pinned != 0 {
		t.Fatalf("message %d/%d text=%q pinned=%d", user, box, got, pinned)
	}
}

func assertPtsRow(t *testing.T, store *msgdao.Mysql, ctx context.Context, user int64, pts, count int32, ids []int32) *mtproto.Update {
	t.Helper()
	rows, err := store.UserPtsUpdatesDAO.SelectByGtPts(ctx, user, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("user %d pts rows %d", user, len(rows))
	}
	row := rows[0]
	if row.Pts != pts || row.PtsCount != count {
		t.Fatalf("user %d stored pts %d/%d, want %d/%d", user, row.Pts, row.PtsCount, pts, count)
	}
	var upd mtproto.Update
	if err = jsonx.UnmarshalFromString(row.UpdateData, &upd); err != nil {
		t.Fatalf("user %d update %s: %v", user, row.UpdateData, err)
	}
	if upd.GetPinned() || upd.GetPredicateName() != mtproto.Predicate_updatePinnedMessages {
		t.Fatalf("user %d update %#v", user, &upd)
	}
	if !sameIDs(upd.GetMessages(), ids) || upd.GetPts_INT32() != pts || upd.GetPtsCount() != count {
		t.Fatalf("user %d update ids %v pts %d/%d", user, upd.GetMessages(), upd.GetPts_INT32(), upd.GetPtsCount())
	}
	return &upd
}

func updatesOf(t *testing.T, ups *mtproto.Updates) *mtproto.Update {
	t.Helper()
	if ups == nil || len(ups.GetUpdates()) != 1 {
		t.Fatalf("updates %#v", ups)
	}
	return ups.GetUpdates()[0]
}

func assertSameUpdate(t *testing.T, pushed, stored *mtproto.Update, peer int64) {
	t.Helper()
	if pushed.GetPinned() || pushed.GetPts_INT32() != stored.GetPts_INT32() || pushed.GetPtsCount() != stored.GetPtsCount() {
		t.Fatalf("pushed %#v stored %#v", pushed, stored)
	}
	if !sameIDs(pushed.GetMessages(), stored.GetMessages()) || pushed.GetPeer_PEER().GetUserId() != peer {
		t.Fatalf("pushed ids %v peer %d", pushed.GetMessages(), pushed.GetPeer_PEER().GetUserId())
	}
}

func sameIDs(got, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int32]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}
	return true
}
