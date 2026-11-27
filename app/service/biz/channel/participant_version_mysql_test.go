package channel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestChannelMemberVersionMySQL runs on the inbox-mysql job. A missing DSN
// skips locally. TEAMGRAM_REQUIRE_INBOX_MYSQL makes a missing database fail,
// and that job also rejects a SKIP line. This is not an MTProto session.
func TestChannelMemberVersionMySQL(t *testing.T) {
	dsn := channelMySQLDSN(t)
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err = admin.Ping(); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("chan_ver_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`") })
	db, err := sql.Open("mysql", channelSchemaDSN(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec(`CREATE TABLE channel_participants (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role INT NOT NULL DEFAULT 0,
  state INT NOT NULL DEFAULT 0,
  admin_rights INT NOT NULL DEFAULT 0,
  banned_rights BIGINT NOT NULL DEFAULT 0,
  rank_name VARCHAR(16) NOT NULL DEFAULT '',
  inviter_user_id BIGINT NOT NULL DEFAULT 0,
  is_bot TINYINT NOT NULL DEFAULT 0,
  date2 INT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO channel_participants (channel_id, user_id, role, state, date2) VALUES (1, 21, 0, 0, 10)`); err != nil {
		t.Fatal(err)
	}
	svc, err := OpenMySQL(channelSchemaDSN(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = OpenMySQL(channelSchemaDSN(dsn, schema)); err != nil {
		t.Fatalf("second schema upgrade: %v", err)
	}
	var columns int
	if err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA=? AND TABLE_NAME='channel_participants' AND COLUMN_NAME='row_version'`, schema).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 1 {
		t.Fatalf("row_version columns=%d", columns)
	}
	var oldVersion int32
	if err = db.QueryRow(`SELECT row_version FROM channel_participants WHERE channel_id=1 AND user_id=21`).Scan(&oldVersion); err != nil {
		t.Fatal(err)
	}
	if oldVersion != 0 {
		t.Fatalf("upgraded legacy row version=%d", oldVersion)
	}
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Version", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	chID := created.Chats[0].Id
	if err = svc.store.PutParticipant(ctx, &Participant{
		ChannelID: chID, UserID: 21, Role: RoleMember, State: StateNormal, Date: 10, rowVersion: 9,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.store.GetParticipant(ctx, chID, 21)
	if err != nil || got == nil || got.rowVersion != 1 {
		t.Fatalf("mysql insert kept the caller version %+v %v", got, err)
	}
	stale := *got
	got.Rank = "live"
	if err = svc.store.PutParticipant(ctx, got); err != nil {
		t.Fatal(err)
	}
	stale.State = StateLeft
	if err = svc.store.PutParticipant(ctx, &stale); !errors.Is(err, errParticipantChanged) {
		t.Fatalf("stale mysql write err=%v", err)
	}
	after, err := svc.store.GetParticipant(ctx, chID, 21)
	if err != nil || after.State != StateNormal || after.Rank != "live" || after.rowVersion != 2 {
		t.Fatalf("stale mysql write changed %+v %v", after, err)
	}
	first, err := svc.store.GetFeatures(ctx, chID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.store.GetFeatures(ctx, chID)
	if err != nil {
		t.Fatal(err)
	}
	if first.JoinTop == nil {
		first.JoinTop = map[int64]int32{}
	}
	if second.JoinTop == nil {
		second.JoinTop = map[int64]int32{}
	}
	first.JoinTop[21] = 2
	second.JoinTop[22] = 3
	second.SlowmodeSeconds = 15
	if err = svc.store.SaveFeatures(ctx, chID, second); err != nil {
		t.Fatal(err)
	}
	if err = svc.store.SaveFeatures(ctx, chID, first); err != nil {
		t.Fatal(err)
	}
	feat, err := svc.mutableFeatures(ctx, chID)
	if err != nil || feat.JoinTop[21] != 2 || feat.JoinTop[22] != 3 || feat.SlowmodeSeconds != 15 {
		t.Fatalf("mysql stale feature merge %+v %v", feat, err)
	}
}
