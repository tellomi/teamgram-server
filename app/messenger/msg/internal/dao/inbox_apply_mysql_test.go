package dao

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"

	_ "github.com/go-sql-driver/mysql"
)

// These ids are fixtures for this file. They are not accounts.
const (
	applyUser   int64 = 910442041
	applyPeer   int64 = 910442042
	applyRandom int64 = 910442043
	applyBox    int32 = 17
	applyGroup  int64 = 910442044
)

func TestInboxCrashBeforeUnreadAddsOnce(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, applyBox, "hello-17")
	if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		return recordInboxApplyTx(tx, inboxApplyRow{
			UserID: applyUser, BoxID: applyBox, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
			UnreadDelta: 1, MentionsDelta: 0, Pts: 5, PtsCount: 1, Date2: 10,
		})
	}); err != nil {
		t.Fatal(err)
	}
	insertDialog(t, d, applyUser, mtproto.PEER_USER, applyPeer, 9, 0, 4)
	insertPts(t, d, applyUser, 5, applyBox, "hello-17")

	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 4 || top != 9 {
		t.Fatalf("before replay unread=%d top=%d", unread, top)
	}
	if _, err := d.FinishInboxDelivery(ctx, applyUser, applyBox); err != nil {
		t.Fatal(err)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 5 || top != applyBox {
		t.Fatalf("after replay unread=%d top=%d, want 5 and %d", unread, top, applyBox)
	}
	if _, err := d.FinishInboxDelivery(ctx, applyUser, applyBox); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errc := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.FinishInboxDelivery(ctx, applyUser, applyBox); err != nil {
				errc <- err
			}
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatal(err)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 5 || top != applyBox {
		t.Fatalf("concurrent replay unread=%d top=%d", unread, top)
	}
	if n := ptsRows(t, d, applyUser); n != 1 {
		t.Fatalf("pts rows = %d, want 1", n)
	}
	if gaps, err := d.CheckPtsContinuity(ctx, applyUser, 4); err != nil || len(gaps) != 0 {
		t.Fatalf("difference gaps %v err %v", gaps, err)
	}
}

func TestInboxReplayRespectsReadDeleteAndOlderTop(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	const (
		readBox  int32 = 21
		deadBox  int32 = 22
		olderBox int32 = 15
	)
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, readBox, "read-me")
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, deadBox, "gone")
	insertLiveMessage(t, d, applyUser, applyPeer, mtproto.PEER_USER, olderBox, "older")
	for _, box := range []int32{readBox, deadBox, olderBox} {
		if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
			return recordInboxApplyTx(tx, inboxApplyRow{
				UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_USER, PeerID: applyPeer,
				UnreadDelta: 1, Pts: box, PtsCount: 1, Date2: int64(box),
			})
		}); err != nil {
			t.Fatal(err)
		}
	}
	insertDialog(t, d, applyUser, mtproto.PEER_USER, applyPeer, 30, readBox, 0)
	if _, err := d.DB.Exec(ctx, `UPDATE messages SET deleted = 1, message = '' WHERE user_id = ? AND user_message_box_id = ?`, applyUser, deadBox); err != nil {
		t.Fatal(err)
	}

	if _, err := d.FinishInboxDelivery(ctx, applyUser, readBox); err != nil {
		t.Fatal(err)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 0 || top != 30 {
		t.Fatalf("read race unread=%d top=%d", unread, top)
	}
	if _, err := d.FinishInboxDelivery(ctx, applyUser, deadBox); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := queryMessage(t, d, applyUser, deadBox, &body); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		t.Fatalf("deleted body restored to %q", body)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 0 || top != 30 {
		t.Fatalf("delete race unread=%d top=%d", unread, top)
	}
	deleted := dataobjectMessage(applyUser, applyPeer, deadBox, "")
	deleted.Deleted = true
	if _, _, apply, err := d.replayExistingInbox(ctx, deleted); err != nil {
		t.Fatal(err)
	} else if apply.Push {
		t.Fatal("deleted replay pushed the message")
	}
	if _, err := d.DB.Exec(ctx, `UPDATE dialogs SET read_inbox_max_id = 0, unread_count = 0, top_message = 30 WHERE user_id = ? AND peer_id = ?`, applyUser, applyPeer); err != nil {
		t.Fatal(err)
	}
	if _, err := d.FinishInboxDelivery(ctx, applyUser, olderBox); err != nil {
		t.Fatal(err)
	}
	if unread, top := dialogState(t, d, applyUser, mtproto.PEER_USER, applyPeer); unread != 1 || top != 30 {
		t.Fatalf("older live replay unread=%d top=%d, want unread 1 and top 30", unread, top)
	}
}

func TestGroupAndV3PtsReplay(t *testing.T) {
	d := openInboxApplyDB(t)
	ctx := context.Background()
	const box int32 = 8
	insertLiveMessage(t, d, applyUser, applyGroup, mtproto.PEER_CHAT, box, "group-8")
	if err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		return recordInboxApplyTx(tx, inboxApplyRow{
			UserID: applyUser, BoxID: box, PeerType: mtproto.PEER_CHAT, PeerID: applyGroup,
			UnreadDelta: 1, MentionsDelta: 1, Pts: 3, PtsCount: 1, Date2: 8,
		})
	}); err != nil {
		t.Fatal(err)
	}
	insertDialog(t, d, applyUser, mtproto.PEER_CHAT, applyGroup, 2, 0, 0)
	if _, err := d.FinishInboxDelivery(ctx, applyUser, box); err != nil {
		t.Fatal(err)
	}
	if _, err := d.FinishInboxDelivery(ctx, applyUser, box); err != nil {
		t.Fatal(err)
	}
	var unread, mentions, top int32
	var row struct {
		Unread   int32 `db:"unread_count"`
		Mentions int32 `db:"unread_mentions_count"`
		Top      int32 `db:"top_message"`
	}
	if err := d.DB.QueryRow(ctx, &row, `SELECT unread_count, unread_mentions_count, top_message FROM dialogs WHERE user_id = ? AND peer_type = ? AND peer_id = ?`, applyUser, mtproto.PEER_CHAT, applyGroup); err != nil {
		t.Fatal(err)
	}
	unread, mentions, top = row.Unread, row.Mentions, row.Top
	if unread != 1 || mentions != 1 || top != box {
		t.Fatalf("group unread=%d mentions=%d top=%d", unread, mentions, top)
	}

	const ptsUser int64 = 910442045
	var cur int32
	d.ptsCurrent = func(context.Context, int64) int32 { return cur }
	d.ptsNext = func(context.Context, int64) int32 { cur++; return cur }
	var burned, reused int32
	err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		var err error
		burned, err = d.chooseInboxPts(ctx, tx, ptsUser)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		var err error
		reused, err = d.chooseInboxPts(ctx, tx, ptsUser)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
			ptsUser, reused, 1, mtproto.PTS_UPDATE_NEW_MESSAGE, fmt.Sprintf(`{"predicate_name":"updateNewMessage","pts_INT32":%d,"pts_count":1,"message_MESSAGE":{"predicate_name":"message","id":%d,"message":"v3"}}`, reused, reused), 1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if burned != 1 || reused != 1 {
		t.Fatalf("v3 pts burned=%d reused=%d, want the same unwritten pts", burned, reused)
	}
	if gaps, err := d.CheckPtsContinuity(ctx, ptsUser, 0); err != nil || len(gaps) != 0 {
		t.Fatalf("v3 difference gaps %v err %v", gaps, err)
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
			dialog_id1 BIGINT NOT NULL DEFAULT 0,
			dialog_id2 BIGINT NOT NULL DEFAULT 0,
			dialog_message_id BIGINT NOT NULL DEFAULT 0,
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
			date2 BIGINT NOT NULL DEFAULT 0,
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
			unread_count INT NOT NULL DEFAULT 0,
			unread_mentions_count INT NOT NULL DEFAULT 0,
			read_inbox_max_id INT NOT NULL DEFAULT 0,
			draft_message_data JSON NOT NULL,
			date2 BIGINT NOT NULL DEFAULT 0,
			deleted TINYINT NOT NULL DEFAULT 0,
			ttl_period INT NOT NULL DEFAULT 0,
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
		`CREATE TABLE inbox_pts_lock (
			user_id BIGINT NOT NULL PRIMARY KEY
		)`,
	}
	for _, stmt := range stmts {
		if _, err = db.Exec(context.Background(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	return &Dao{Mysql: NewMysqlDao(db, 0)}
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

func insertPts(t *testing.T, d *Dao, user int64, pts, box int32, text string) {
	t.Helper()
	raw := fmt.Sprintf(`{"predicate_name":"updateNewMessage","pts_INT32":%d,"pts_count":1,"message_MESSAGE":{"predicate_name":"message","id":%d,"message":%q}}`, pts, box, text)
	if _, err := d.DB.Exec(context.Background(), `INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		user, pts, 1, mtproto.PTS_UPDATE_NEW_MESSAGE, raw, 1); err != nil {
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

func ptsRows(t *testing.T, d *Dao, user int64) int {
	t.Helper()
	var row struct {
		N int `db:"n"`
	}
	if err := d.DB.QueryRow(context.Background(), &row, `SELECT COUNT(*) AS n FROM user_pts_updates WHERE user_id = ?`, user); err != nil {
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
