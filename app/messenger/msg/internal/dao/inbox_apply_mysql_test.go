package dao

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/stores/redis"

	_ "github.com/go-sql-driver/mysql"
)

const (
	applyUser   int64 = 910442041
	applyPeer   int64 = 910442042
	applyRandom int64 = 910442043
	applyGroup  int64 = 910442044
)

func TestInboxUnreadCommitsWithTheMessage(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	const box int32 = 17
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, box, "hello-17")
	if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		_, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 17,
		}, textMessage(box, "hello-17"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 1 || top != box {
		t.Fatalf("committed unread=%d top=%d", unread, top)
	}
	if applied := applyFlag(t, d, applyUser, box); applied != 1 {
		t.Fatalf("applied=%d, want 1 in the same commit", applied)
	}
	row := dataobjectMessage(applyUser, applyPeer, box, "hello-17")
	if _, _, apply, err := d.replayExistingInbox(ctx, row); err != nil {
		t.Fatal(err)
	} else if !apply.Push || apply.PersistPts {
		t.Fatalf("replay %#v", apply)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 1 || top != box {
		t.Fatalf("replay unread=%d top=%d", unread, top)
	}
}

func TestInboxRollbackLeavesUnreadForTheOldProcess(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	const box int32 = 18
	err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.Exec(`INSERT INTO messages (user_id, user_message_box_id, sender_user_id, peer_type, peer_id, random_id, message_data, message, deleted) VALUES (?,?,?,?,?,?,?,?,0)`,
			applyUser, box, applyPeer, mtproto.PEER_USER, applyPeer, applyRandom, `{"predicate_name":"message"}`, "rolled"); err != nil {
			return err
		}
		if _, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 18,
		}, textMessage(box, "rolled")); err != nil {
			return err
		}
		return fmt.Errorf("process exit before commit")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=?`, applyUser); n != 0 {
		t.Fatalf("rolled-back message rows=%d", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM inbox_apply WHERE user_id=?`, applyUser); n != 0 {
		t.Fatalf("rolled-back apply rows=%d", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM inbox_random WHERE user_id=?`, applyUser); n != 0 {
		t.Fatalf("rolled-back random rows=%d", n)
	}
	// No inbox_random row, so the master insert still stores the message and adds unread.
	if err = sendInboxLikeMaster(ctx, d, applyPeer, applyUser, box, applyRandom, "fresh"); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=? AND user_message_box_id=?`, applyUser, box); n != 1 {
		t.Fatalf("master first insert rows=%d", n)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 1 || top != box {
		t.Fatalf("master first unread=%d top=%d", unread, top)
	}
}

// TestMasterRedeliveryDoesNotSecondInsert runs the master sendMessageToInbox
// persistence: a new box id, InsertOrReturnIdTx, then InsertOrUpdateForward
// outside the transaction. origin/master does not look up random_id.
func TestMasterRedeliveryDoesNotSecondInsert(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	const stored int32 = 17
	const redelivery int32 = 99
	if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		if _, _, err := d.MessagesDAO.InsertOrReturnIdTx(tx, inboxMessageDO(applyUser, applyPeer, stored, applyRandom, "hello")); err != nil {
			return err
		}
		if err := rememberInboxRandom(tx, applyUser, applyPeer, applyRandom, stored); err != nil {
			return err
		}
		_, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: stored, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 17,
		}, textMessage(stored, "hello"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := sendInboxLikeMaster(ctx, d, applyPeer, applyUser, redelivery, applyRandom, "hello")
	if err == nil {
		t.Fatal("master redelivery inserted again")
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=?`, applyUser); n != 1 {
		t.Fatalf("message rows=%d", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM messages WHERE user_id=? AND user_message_box_id=?`, applyUser, redelivery); n != 0 {
		t.Fatal("redelivery box was stored")
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 1 || top != stored {
		t.Fatalf("after master redelivery unread=%d top=%d", unread, top)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM user_pts_updates WHERE user_id=?`, applyUser); n != 1 {
		t.Fatalf("pts rows=%d, master handler persists only after a nil error", n)
	}
}

func TestInboxReadDeleteAndCache(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	d.CachedConn = sqlc.NewNodeConn(d.DB, rds)
	const box int32 = 21
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, box, "read-me")
	insertDialog(t, d, applyUser, mtproto.PEER_USER, applyPeer, 30, box, 0)
	key := dialogCacheKey(applyUser, mtproto.PEER_USER, applyPeer)
	if err = d.CachedConn.SetCache(ctx, key, "unread=0"); err != nil {
		t.Fatal(err)
	}
	if err = d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		_, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 21,
		}, textMessage(box, "read-me"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	d.invalidateDialogKeys(ctx, key)
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 0 || top != 30 {
		t.Fatalf("read cursor unread=%d top=%d", unread, top)
	}
	var cached string
	if err = d.CachedConn.GetCache(ctx, key, &cached); err == nil {
		t.Fatalf("stale dialog cache still present: %q", cached)
	}

	const dead int32 = 22
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, dead, "gone")
	if err = d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		_, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: dead, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 22,
		}, textMessage(dead, "gone"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = d.DB.Exec(ctx, `UPDATE messages SET deleted=1, message='' WHERE user_id=? AND user_message_box_id=?`, applyUser, dead); err != nil {
		t.Fatal(err)
	}
	if _, err = d.DB.Exec(ctx, `UPDATE dialogs SET top_message=9, unread_count=0 WHERE user_id=? AND peer_id=?`, applyUser, applyPeer); err != nil {
		t.Fatal(err)
	}
	deleted := dataobjectMessage(applyUser, applyPeer, dead, "")
	deleted.Deleted = true
	if _, _, apply, err := d.replayExistingInbox(ctx, deleted); err != nil {
		t.Fatal(err)
	} else if apply.Push {
		t.Fatal("deleted replay pushed")
	}
	body := ""
	if err = queryMessage(t, d, applyUser, dead, &body); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		t.Fatalf("body restored %q", body)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 0 || top != 9 {
		t.Fatalf("after delete unread=%d top=%d", unread, top)
	}
}

func TestGroupUnreadAndCrossRPCPts(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	var cur atomic.Int32
	d.ptsNext = func(context.Context, int64) int32 { return cur.Add(1) }
	const box int32 = 8
	insertLiveMessage(t, d, applyUser, applyGroup, mtproto.PEER_CHAT, box, "group-8")
	var got int32
	if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		pts, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_CHAT, PeerID: applyGroup,
			UnreadDelta: 1, MentionsDelta: 1, Date2: 8,
		}, textMessage(box, "group-8"))
		got = pts
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var row struct {
		Unread   int32 `db:"unread_count"`
		Mentions int32 `db:"unread_mentions_count"`
		Top      int32 `db:"top_message"`
	}
	if err := d.DB.QueryRow(ctx, &row, `SELECT unread_count, unread_mentions_count, top_message FROM dialogs WHERE user_id=? AND peer_type=? AND peer_id=?`, applyUser, mtproto.PEER_CHAT, applyGroup); err != nil {
		t.Fatal(err)
	}
	if row.Unread != 1 || row.Mentions != 1 || row.Top != box || got != 1 {
		t.Fatalf("group unread=%d mentions=%d top=%d pts=%d", row.Unread, row.Mentions, row.Top, got)
	}

	// A read/edit style caller takes the next pts directly and writes its own row.
	readPts := d.nextUserPts(ctx, applyUser)
	if _, err := d.DB.Exec(ctx, `INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		applyUser, readPts, 1, 2, fmt.Sprintf(`{"predicate_name":"updateReadHistoryInbox","pts_INT32":%d,"pts_count":1}`, readPts), 2); err != nil {
		t.Fatal(err)
	}
	if gaps, err := d.CheckPtsContinuity(ctx, applyUser, 0); err != nil || len(gaps) != 0 {
		t.Fatalf("committed pts gaps %v err %v", gaps, err)
	}
	if readPts != got+1 {
		t.Fatalf("read pts=%d inbox pts=%d", readPts, got)
	}
}

func TestPtsFaultDoesNotStealAnotherRPC(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	var cur atomic.Int32
	d.ptsNext = func(context.Context, int64) int32 { return cur.Add(1) }
	const user int64 = 910442045
	burned := d.nextUserPts(ctx, user)
	readPts := d.nextUserPts(ctx, user)
	if _, err := d.DB.Exec(ctx, `INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		user, readPts, 1, 2, fmt.Sprintf(`{"predicate_name":"updateReadHistoryInbox","pts_INT32":%d,"pts_count":1}`, readPts), 1); err != nil {
		t.Fatal(err)
	}
	const box int32 = 4
	insertLiveMessage(t, d, user, applyPeer, mtproto.PEER_USER, box, "retry")
	var retryPts int32
	if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		pts, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: user, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 4,
		}, textMessage(box, "retry"))
		retryPts = pts
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if burned == readPts || retryPts == readPts || retryPts == burned {
		t.Fatalf("burned=%d read=%d retry=%d", burned, readPts, retryPts)
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM user_pts_updates WHERE user_id=? AND pts=?`, user, burned); n != 0 {
		t.Fatal("burned pts was written")
	}
	if n := countRows(t, d, `SELECT COUNT(*) AS n FROM user_pts_updates WHERE user_id=? AND pts=? AND update_type=2`, user, readPts); n != 1 {
		t.Fatal("read pts row was replaced")
	}
	var stored struct {
		Pts   int32 `db:"pts"`
		Count int32 `db:"pts_count"`
	}
	if err := d.DB.QueryRow(ctx, &stored, `SELECT pts, pts_count FROM user_pts_updates WHERE user_id=? AND pts=?`, user, retryPts); err != nil {
		t.Fatal(err)
	}
	if stored.Pts != retryPts || stored.Count != 1 || (retryPts-stored.Count) != retryPts-1 {
		t.Fatalf("stored %+v", stored)
	}
	// SelectByGtPts returns later rows, so the read update is still reachable
	// from the pts before it. The burned number is not written and is not reused.
	if gaps, err := d.CheckPtsContinuity(ctx, user, readPts-1); err != nil || len(gaps) != 0 {
		t.Fatalf("read update not applicable: %v %v", gaps, err)
	}
}

func TestConcurrentInboxApplyDoesNotDeadlock(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var cur atomic.Int32
	d.ptsNext = func(context.Context, int64) int32 { return cur.Add(1) }
	var wg sync.WaitGroup
	errc := make(chan error, 2)
	for _, box := range []int32{31, 32} {
		box := box
		insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, box, "c")
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
				_, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
					UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
					UnreadDelta: 1, Date2: int64(box),
				}, textMessage(box, "c"))
				return err
			})
			if err != nil {
				errc <- err
			}
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatal(err)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 2 || top != 32 {
		t.Fatalf("concurrent unread=%d top=%d", unread, top)
	}
	if gaps, err := d.CheckPtsContinuity(ctx, applyUser, 0); err != nil || len(gaps) != 0 {
		t.Fatalf("gaps %v err %v", gaps, err)
	}
}

func TestMissingInboxApplyTableFailsTheInsert(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	if _, err := d.DB.Exec(ctx, `DROP TABLE inbox_apply`); err != nil {
		t.Fatal(err)
	}
	const box int32 = 3
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, box, "x")
	err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		_, _, err := d.applyInboxInTx(ctx, tx, inboxApplyRow{
			UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, Date2: 3,
		}, textMessage(box, "x"))
		return err
	})
	if err == nil {
		t.Fatal("missing inbox_apply reported success")
	}
}

func openInboxApplyDB(t *testing.T) *Dao {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	admin, err := sqlx.Open(&sqlx.Config{DSN: dsn})
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	if _, err = admin.Exec(context.Background(), "SELECT 1"); err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	const schema = "teamgram_inbox_apply_910442"
	if _, err = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(context.Background(), "CREATE DATABASE "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+schema)
	})
	db, err := sqlx.Open(&sqlx.Config{DSN: dsnWithSchema(dsn, schema)})
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE messages (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			user_message_box_id INT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			peer_type INT NOT NULL,
			peer_id BIGINT NOT NULL,
			random_id BIGINT NOT NULL DEFAULT 0,
			message_data JSON NULL,
			message VARCHAR(6000) NOT NULL DEFAULT '',
			mentioned TINYINT NOT NULL DEFAULT 0,
			media_unread TINYINT NOT NULL DEFAULT 0,
			pinned TINYINT NOT NULL DEFAULT 0,
			dialog_id1 BIGINT NOT NULL DEFAULT 0,
			dialog_id2 BIGINT NOT NULL DEFAULT 0,
			dialog_message_id BIGINT NOT NULL DEFAULT 0,
			message_filter_type INT NOT NULL DEFAULT 0,
			saved_peer_type INT NOT NULL DEFAULT 0,
			saved_peer_id BIGINT NOT NULL DEFAULT 0,
			date2 BIGINT NOT NULL DEFAULT 0,
			ttl_period INT NOT NULL DEFAULT 0,
			deleted TINYINT NOT NULL DEFAULT 0,
			UNIQUE KEY box_id (user_id, user_message_box_id)
		)`,
		`CREATE TABLE dialogs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			peer_type INT NOT NULL,
			peer_id BIGINT NOT NULL,
			peer_dialog_id BIGINT NOT NULL,
			top_message INT NOT NULL DEFAULT 0,
			pinned_msg_id INT NOT NULL DEFAULT 0,
			unread_count INT NOT NULL DEFAULT 0,
			unread_mentions_count INT NOT NULL DEFAULT 0,
			read_inbox_max_id INT NOT NULL DEFAULT 0,
			draft_message_data JSON NOT NULL,
			date2 BIGINT NOT NULL DEFAULT 0,
			UNIQUE KEY peer (user_id, peer_type, peer_id)
		)`,
		`CREATE TABLE user_pts_updates (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			pts INT NOT NULL,
			pts_count INT NOT NULL,
			update_type TINYINT NOT NULL DEFAULT 0,
			update_data JSON NOT NULL,
			date2 BIGINT NOT NULL
		)`,
		`CREATE TABLE inbox_apply (
			user_id BIGINT NOT NULL,
			box_id INT NOT NULL,
			peer_type INT NOT NULL,
			peer_id BIGINT NOT NULL,
			unread_delta INT NOT NULL,
			mentions_delta INT NOT NULL,
			pts INT NOT NULL,
			pts_count INT NOT NULL,
			date2 BIGINT NOT NULL,
			applied TINYINT NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, box_id)
		)`,
		`CREATE TABLE inbox_random (
			user_id BIGINT NOT NULL,
			random_id BIGINT NOT NULL,
			sender_user_id BIGINT NOT NULL,
			box_id INT NOT NULL,
			PRIMARY KEY (user_id, random_id, sender_user_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err = db.Exec(context.Background(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(context.Background(), `CREATE TRIGGER messages_inbox_random_guard
BEFORE INSERT ON messages
FOR EACH ROW
BEGIN
  IF NEW.random_id <> 0 AND EXISTS (
    SELECT 1 FROM inbox_random
    WHERE user_id = NEW.user_id AND random_id = NEW.random_id AND sender_user_id = NEW.sender_user_id
  ) THEN
    SIGNAL SQLSTATE '23000' SET MYSQL_ERRNO = 1062, MESSAGE_TEXT = 'Duplicate inbox random_id';
  END IF;
END`); err != nil {
		t.Fatal(err)
	}
	d := &Dao{Mysql: NewMysqlDao(db, 0)}
	var n atomic.Int32
	d.ptsNext = func(context.Context, int64) int32 { return n.Add(1) }
	return d
}

func insertLiveMessage(t *testing.T, d *Dao, user, peer int64, peerType, box int32, text string) {
	t.Helper()
	data := fmt.Sprintf(`{"predicate_name":"message","id":%d,"message":%q}`, box, text)
	if _, err := d.DB.Exec(context.Background(), `INSERT INTO messages (user_id, user_message_box_id, sender_user_id, peer_type, peer_id, random_id, message_data, message, deleted) VALUES (?,?,?,?,?,?,?,?,0)`,
		user, box, peer, peerType, peer, applyRandom, data, text); err != nil {
		t.Fatal(err)
	}
}

func insertDialog(t *testing.T, d *Dao, user int64, peerType int32, peer int64, top, read, unread int32) {
	t.Helper()
	if _, err := d.DB.Exec(context.Background(), `INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, top_message, read_inbox_max_id, unread_count, draft_message_data, date2) VALUES (?,?,?,?,?,?,?,'null',1)`,
		user, peerType, peer, mtproto.MakePeerDialogId(peerType, peer), top, read, unread); err != nil {
		t.Fatal(err)
	}
}

func dialogState(t *testing.T, d *Dao, user int64, peerType int32, peer int64) (unread, top int32) {
	t.Helper()
	var row struct {
		Unread int32 `db:"unread_count"`
		Top    int32 `db:"top_message"`
	}
	if err := d.DB.QueryRow(context.Background(), &row, `SELECT unread_count, top_message FROM dialogs WHERE user_id = ? AND peer_type = ? AND peer_id = ?`, user, peerType, peer); err != nil {
		t.Fatal(err)
	}
	return row.Unread, row.Top
}

func applyFlag(t *testing.T, d *Dao, user int64, box int32) int32 {
	t.Helper()
	var row struct {
		Applied int32 `db:"applied"`
	}
	if err := d.DB.QueryRow(context.Background(), &row, `SELECT applied FROM inbox_apply WHERE user_id=? AND box_id=?`, user, box); err != nil {
		t.Fatal(err)
	}
	return row.Applied
}

func countRows(t *testing.T, d *Dao, query string, args ...interface{}) int {
	t.Helper()
	var row struct {
		N int `db:"n"`
	}
	if err := d.DB.QueryRow(context.Background(), &row, query, args...); err != nil {
		t.Fatal(err)
	}
	return row.N
}

func queryMessage(t *testing.T, d *Dao, user int64, box int32, body *string) error {
	t.Helper()
	var row struct {
		Message string `db:"message"`
	}
	if err := d.DB.QueryRow(context.Background(), &row, `SELECT message FROM messages WHERE user_id = ? AND user_message_box_id = ?`, user, box); err != nil {
		return err
	}
	*body = row.Message
	return nil
}

func dataobjectMessage(user, peer int64, box int32, text string) *dataobject.MessagesDO {
	return &dataobject.MessagesDO{
		UserId:           user,
		UserMessageBoxId: box,
		SenderUserId:     peer,
		PeerType:         mtproto.PEER_USER,
		PeerId:           peer,
		Message:          text,
		MessageData:      fmt.Sprintf(`{"predicate_name":"message","id":%d,"message":%q}`, box, text),
	}
}

// sendInboxLikeMaster is the persistence in origin/master sendMessageToInbox
// at 0fe2b9e56. It allocates the given box id, inserts through
// InsertOrReturnIdTx, and only then adds unread with InsertOrUpdateForward.
// It does not look up random_id. Pts is allocated only after that returns nil,
// which is when the master handler calls persistPtsUpdate.
func sendInboxLikeMaster(ctx context.Context, d *Dao, fromID, toID int64, boxID int32, randomID int64, text string) error {
	err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		_, _, err := d.MessagesDAO.InsertOrReturnIdTx(tx, inboxMessageDO(toID, fromID, boxID, randomID, text))
		return err
	})
	if err != nil {
		return err
	}
	_, _, err = d.DialogsDAO.InsertOrUpdateForward(ctx, &dataobject.DialogsDO{
		UserId:           toID,
		PeerType:         mtproto.PEER_USER,
		PeerId:           fromID,
		PeerDialogId:     mtproto.MakePeerDialogId(mtproto.PEER_USER, fromID),
		TopMessage:       boxID,
		UnreadCount:      1,
		DraftMessageData: "null",
		Date2:            int64(boxID),
	})
	if err != nil {
		return err
	}
	pts := d.nextUserPts(ctx, toID)
	_, err = d.DB.Exec(ctx, `INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		toID, pts, 1, mtproto.PTS_UPDATE_NEW_MESSAGE, fmt.Sprintf(`{"predicate_name":"updateNewMessage","pts_INT32":%d,"pts_count":1}`, pts), int64(boxID))
	return err
}

func inboxMessageDO(userID, senderID int64, boxID int32, randomID int64, text string) *dataobject.MessagesDO {
	return &dataobject.MessagesDO{
		UserId:           userID,
		UserMessageBoxId: boxID,
		SenderUserId:     senderID,
		PeerType:         mtproto.PEER_USER,
		PeerId:           senderID,
		RandomId:         randomID,
		Message:          text,
		MessageData:      fmt.Sprintf(`{"predicate_name":"message","id":%d,"message":%q}`, boxID, text),
		Date2:            int64(boxID),
	}
}

func textMessage(box int32, text string) *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{Id: box, Message: text}).To_Message()
}
