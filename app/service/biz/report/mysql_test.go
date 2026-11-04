package report

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMySQLSurvivesRestartAndBackfill(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
	}
	svc, err := OpenMySQL(dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	ctx := context.Background()
	const reporter int64 = 88051105
	t.Cleanup(func() {
		if db, ok := svc.store.(*mysqlStore); ok {
			_, _ = db.db.ExecContext(context.Background(), `DELETE FROM reports WHERE reporter_user_id=?`, reporter)
			_, _ = db.db.ExecContext(context.Background(), `DELETE FROM story_reports WHERE reporter_user_id=?`, reporter)
		}
	})
	store := svc.store.(*mysqlStore)
	_, _ = store.db.ExecContext(ctx, `DELETE FROM reports WHERE reporter_user_id=?`, reporter)
	kinds := []Submission{
		{Source: SourceStory, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: 1, SubjectUserID: 1, ObjectIDs: []int32{2}, Reason: "spam"},
		{Source: SourceMessages, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: 2, SubjectUserID: 2, ObjectIDs: []int32{3}, Reason: "spam"},
		{Source: SourceReaction, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: 2, ObjectIDs: []int32{3}, ReactionPeerID: 4, SubjectUserID: 4},
		{Source: SourcePeer, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: 5, SubjectUserID: 5, Reason: "spam"},
		{Source: SourceProfilePhoto, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: 6, SubjectUserID: 6, PhotoID: 7, Reason: "spam"},
		{Source: SourceSpam, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: 8, SubjectUserID: 8, Reason: "spam"},
		{Source: SourceChannelSpam, Reporter: reporter, PeerType: mtproto.PEER_CHANNEL, PeerID: 9, SubjectUserID: 10, ObjectIDs: []int32{1}, Reason: "spam"},
	}
	for _, in := range kinds {
		if _, err = svc.Submit(ctx, in); err != nil {
			t.Fatalf("%s: %v", in.Source, err)
		}
	}
	reopened, err := OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, row := range got {
		if row.Reporter == reporter {
			seen[row.Source]++
		}
	}
	if len(seen) != len(kinds) {
		t.Fatalf("survived %#v", seen)
	}

	db := reopened.store.(*mysqlStore).db
	_, err = db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS story_reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  reporter_user_id BIGINT NOT NULL,
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  story_ids VARCHAR(255) NOT NULL DEFAULT '',
  option_text VARCHAR(64) NOT NULL DEFAULT '',
  message TEXT,
  date2 INT NOT NULL,
  PRIMARY KEY (id)
)`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.ExecContext(ctx, `INSERT INTO story_reports (reporter_user_id, owner_kind, owner_id, story_ids, option_text, message, date2) VALUES (?,?,?,?,?,?,?)`,
		reporter, 1, 42, "6,5", "spam", "old", 11)
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := res.LastInsertId()
	n, err := BackfillStoryReports(ctx, db, reopened.store)
	if err != nil || n != 1 {
		t.Fatalf("backfill %d %v", n, err)
	}
	n, err = BackfillStoryReports(ctx, db, reopened.store)
	if err != nil || n != 0 {
		t.Fatalf("second backfill %d %v", n, err)
	}
	all, err := reopened.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range all {
		if row.LegacyStoryID == legacy {
			found = row.TargetKey == "story:2:42:5,6" && row.Source == SourceStory
		}
	}
	if !found {
		t.Fatal("legacy story row missing after reopen")
	}
}
