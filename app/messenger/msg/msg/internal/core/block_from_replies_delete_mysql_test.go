package core

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bwmarrin/snowflake"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	msgdao "github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/svc"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	idgenhelper "github.com/teamgram/teamgram-server/app/service/idgen"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"google.golang.org/grpc/status"
)

const repliesServiceUser int64 = 1271266957

// TestMsgDeleteMessagesRejectsRepliesPeer is the real peer switch.
// contacts.blockFromReplies used to call this with PEER_USER and the
// @replies user. That call never reaches the messages table.
func TestMsgDeleteMessagesRejectsRepliesPeer(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &svc.ServiceContext{}).MsgDeleteMessages(&msgpb.TLMsgDeleteMessages{
		UserId:   3,
		PeerType: mtproto.PEER_USER,
		PeerId:   repliesServiceUser,
		Id:       []int32{40, 30},
	})
	if status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("peer user delete %v", err)
	}
}

// TestBlockFromRepliesDeletePersistsCallerRows deletes through MsgDeleteMessages
// the same way messages.deleteMessages does: PEER_EMPTY and the caller's box
// ids. A PEER_USER call leaves every row. The empty-peer call removes only
// the ids it was given.
func TestBlockFromRepliesDeletePersistsCallerRows(t *testing.T) {
	admin, dsn := openPrivateAdmin(t)
	schema := fmt.Sprintf("bfr_del_%d_%d", os.Getpid(), time.Now().UnixNano())
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
	if _, err = db.Exec(`ALTER TABLE dialogs ADD COLUMN unread_mark TINYINT NOT NULL DEFAULT 0, ADD COLUMN draft_message_data JSON NULL`); err != nil {
		t.Fatal(err)
	}

	const (
		caller  int64 = 910442101
		other   int64 = 910442102
		private int64 = 910442199
	)
	replies := mtproto.MakeDialogId(caller, mtproto.PEER_USER, repliesServiceUser)
	dm := mtproto.MakeDialogId(caller, mtproto.PEER_USER, private)
	otherReplies := mtproto.MakeDialogId(other, mtproto.PEER_USER, repliesServiceUser)
	insertMessage(t, db, caller, 40, replies, repliesServiceUser, repliesServiceUser, "n40", 0, 0)
	insertMessage(t, db, caller, 30, replies, repliesServiceUser, repliesServiceUser, "n30", 0, 0)
	insertMessage(t, db, caller, 80, replies, repliesServiceUser, repliesServiceUser, "n80", 0, 0)
	insertMessage(t, db, caller, 20, replies, repliesServiceUser, repliesServiceUser, "n20", 0, 0)
	insertMessage(t, db, caller, 7, dm, caller, private, "dm", 0, 0)
	insertMessage(t, db, other, 40, otherReplies, repliesServiceUser, repliesServiceUser, "other", 0, 0)
	insertDialog(t, db, caller, repliesServiceUser, 0, 80, 0, 80)
	insertDialog(t, db, caller, private, 0, 7, 0, 7)
	insertDialog(t, db, other, repliesServiceUser, 0, 40, 0, 40)

	ctx := context.Background()
	core, syncs := newDeleteCore(t, ctx, dbDSN)
	_, err = core.MsgDeleteMessages(&msgpb.TLMsgDeleteMessages{
		UserId:   caller,
		PeerType: mtproto.PEER_USER,
		PeerId:   repliesServiceUser,
		Revoke:   false,
		Id:       []int32{40},
	})
	if status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("before fix path %v", err)
	}
	for _, row := range []struct {
		user int64
		box  int
	}{{caller, 40}, {caller, 30}, {caller, 80}, {caller, 20}, {caller, 7}, {other, 40}} {
		if deletedRow(t, db, row.user, row.box) != 0 {
			t.Fatalf("peer user delete changed %d/%d", row.user, row.box)
		}
	}

	affected, err := core.MsgDeleteMessages(&msgpb.TLMsgDeleteMessages{
		UserId:    caller,
		AuthKeyId: 77,
		PeerType:  mtproto.PEER_EMPTY,
		PeerId:    0,
		Revoke:    false,
		Id:        []int32{40},
	})
	if err != nil {
		t.Fatal(err)
	}
	if affected.GetPtsCount() != 1 || deletedRow(t, db, caller, 40) != 1 {
		t.Fatalf("one id pts %d deleted %d", affected.GetPtsCount(), deletedRow(t, db, caller, 40))
	}
	for _, box := range []int{30, 80, 20, 7} {
		if deletedRow(t, db, caller, box) != 0 {
			t.Fatalf("delete_message removed %d", box)
		}
	}
	if deletedRow(t, db, other, 40) != 0 {
		t.Fatal("delete_message removed the other user's row")
	}

	affected, err = core.MsgDeleteMessages(&msgpb.TLMsgDeleteMessages{
		UserId:    caller,
		AuthKeyId: 77,
		PeerType:  mtproto.PEER_EMPTY,
		PeerId:    0,
		Revoke:    false,
		Id:        []int32{30, 80},
	})
	if err != nil {
		t.Fatal(err)
	}
	if affected.GetPtsCount() != 2 || deletedRow(t, db, caller, 30) != 1 || deletedRow(t, db, caller, 80) != 1 {
		t.Fatalf("history pts %d 30=%d 80=%d", affected.GetPtsCount(), deletedRow(t, db, caller, 30), deletedRow(t, db, caller, 80))
	}
	if deletedRow(t, db, caller, 20) != 0 || deletedRow(t, db, caller, 7) != 0 || deletedRow(t, db, other, 40) != 0 {
		t.Fatal("history delete removed a row outside the id list")
	}
	if len(syncs.push) != 0 {
		t.Fatalf("revoke=false pushed %d", len(syncs.push))
	}
	if len(syncs.notMe) != 2 || syncs.notMe[0].UserId != caller || syncs.notMe[1].UserId != caller {
		t.Fatalf("sync %#v", syncs.notMe)
	}
}

func newDeleteCore(t *testing.T, ctx context.Context, dbDSN string) (*MsgCore, *localSync) {
	t.Helper()
	mr := miniredis.RunT(t)
	sqlDB := sqlx.NewMySQL(&sqlx.Config{DSN: dbDSN})
	store := msgdao.NewMysqlDao(sqlDB, 0)
	rds, err := redis.NewRedis(redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType})
	if err != nil {
		t.Fatal(err)
	}
	node, err := snowflake.NewNode(1)
	if err != nil {
		t.Fatal(err)
	}
	seq := kv.NewStore(kv.KvConf{{
		RedisConf: redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType},
		Weight:    100,
	}})
	idCore := idgenhelper.NewCore(ctx, &idgenhelper.ServiceContext{Dao: &idgenhelper.Dao{Node: node, KV: seq}})
	box := &localSync{}
	dao := &msgdao.Dao{
		Mysql:        store,
		CachedConn:   sqlc.NewNodeConn(sqlDB, rds),
		IDGenClient2: idgen_client.NewIDGenClient2Direct(seqClient{core: idCore}),
		SyncClient:   box,
	}
	return New(ctx, &svc.ServiceContext{Dao: dao}), box
}

func deletedRow(t *testing.T, db *sql.DB, user int64, box int) int {
	t.Helper()
	var deleted int
	if err := db.QueryRow(`SELECT deleted FROM messages WHERE user_id=? AND user_message_box_id=?`, user, box).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	return deleted
}
