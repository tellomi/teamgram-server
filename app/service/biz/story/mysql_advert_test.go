package story

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Local throwaway server only. This test does not read TEAMGRAM_MYSQL_DSN.
// Fixture login for a local throwaway database. It is not a production
// credential. The tests skip when this server is not listening.
const advertTestDSN = "storyadvert:storyadvert@tcp(127.0.0.1:3306)/storyadvert?charset=utf8mb4&parseTime=true&multiStatements=true"

func openAdvertTest(t *testing.T) (*sql.DB, *mysqlStore, int32) {
	t.Helper()
	db, err := sql.Open("mysql", advertTestDSN)
	if err != nil {
		t.Skipf("mysql open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	if err = ensureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS users (
  id BIGINT NOT NULL PRIMARY KEY,
  stories_max_id INT NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatal(err)
	}
	return db, &mysqlStore{db: db}, int32(time.Now().Unix())
}

var advertUserSeq atomic.Int64

func advertUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	id := 800_000_000 + advertUserSeq.Add(1)
	if _, err := db.Exec(`INSERT INTO users (id, stories_max_id) VALUES (?, 0)`, id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stories WHERE owner_id=?`, id)
		_, _ = db.Exec(`DELETE FROM users WHERE id=?`, id)
	})
	return id
}

func storyRow(owner int64, id, expire int32) *Record {
	item := mtproto.MakeTLStoryItem(&mtproto.StoryItem{
		Id: id, Date: expire - 3600, ExpireDate: expire,
		Media:   photoMedia(),
		Privacy: rules("all"),
	}).To_StoryItem()
	return &Record{Owner: Owner{Kind: KindUser, ID: owner}, Sender: owner, Item: item}
}

func userMax(t *testing.T, db *sql.DB, id int64) int32 {
	t.Helper()
	var max int32
	if err := db.QueryRow(`SELECT stories_max_id FROM users WHERE id=?`, id).Scan(&max); err != nil {
		t.Fatal(err)
	}
	return max
}

func TestMySQLRollbackDoesNotKeepStoryOrMax(t *testing.T) {
	db, _, now := openAdvertTest(t)
	ctx := context.Background()
	user := advertUser(t, db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? FOR UPDATE`, user).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
	if err = upsertExec(ctx, tx, storyRow(user, 1, now+3600)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT COUNT(*) FROM stories WHERE owner_kind=? AND owner_id=?`, KindUser, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 || userMax(t, db, user) != 0 {
		t.Fatalf("rolled back story count %d max %d", n, userMax(t, db, user))
	}
}

func TestMySQLCommitSetsMaxAndRepairFixesZero(t *testing.T) {
	db, ms, now := openAdvertTest(t)
	ctx := context.Background()
	user := advertUser(t, db)
	rec := storyRow(user, 4, now+3600)
	max, wrote, err := ms.commitUserAdvert(ctx, user, now, func(tx *sql.Tx) error {
		return upsertExec(ctx, tx, rec)
	})
	if err != nil || !wrote || max != 4 || userMax(t, db, user) != 4 {
		t.Fatalf("commit max %d wrote %v err %v column %d", max, wrote, err, userMax(t, db, user))
	}
	// Crash window: the story row is durable and the advertisement was not updated.
	if err = upsertExec(ctx, db, storyRow(user, 9, now+7200)); err != nil {
		t.Fatal(err)
	}
	if userMax(t, db, user) != 4 {
		t.Fatalf("column changed without the advert transaction: %d", userMax(t, db, user))
	}
	svc := New(ms)
	svc.SetClock(func() int32 { return now })
	svc.repairMySQLAdverts(ctx)
	if got := userMax(t, db, user); got != 9 {
		t.Fatalf("repair max %d, want 9", got)
	}
	quiet := advertUser(t, db)
	svc.repairMySQLAdverts(ctx)
	if userMax(t, db, quiet) != 0 {
		t.Fatalf("user with no stories changed to %d", userMax(t, db, quiet))
	}
	if _, err = db.Exec(`UPDATE users SET stories_max_id=7 WHERE id=?`, user); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE stories SET expire_date=? WHERE owner_id=?`, now-10, user); err != nil {
		t.Fatal(err)
	}
	svc.repairMySQLAdverts(ctx)
	if got := userMax(t, db, user); got != 0 {
		t.Fatalf("expired advert %d, want 0", got)
	}
}

func TestMySQLSecondWriterWaitsAndKeepsNewerMax(t *testing.T) {
	db, ms, now := openAdvertTest(t)
	ctx := context.Background()
	user := advertUser(t, db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? FOR UPDATE`, user).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
	if err = upsertExec(ctx, tx, storyRow(user, 1, now+3600)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `
UPDATE users SET stories_max_id = (
  SELECT IFNULL(MAX(story_id), 0) FROM stories
  WHERE owner_kind=? AND owner_id=? AND deleted=0 AND expire_date>?
) WHERE id=?`, KindUser, user, now, user); err != nil {
		t.Fatal(err)
	}

	finished := make(chan error, 1)
	go func() {
		_, _, err := ms.commitUserAdvert(ctx, user, now, func(tx *sql.Tx) error {
			return upsertExec(ctx, tx, storyRow(user, 2, now+3600))
		})
		finished <- err
	}()
	select {
	case err := <-finished:
		t.Fatalf("second writer finished while the first transaction held the user row: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second writer did not finish after the lock was released")
	}
	if got := userMax(t, db, user); got != 2 {
		t.Fatalf("max %d, want 2", got)
	}
	// A later recompute from a process that only knows the old row still
	// stores the live max, not 0.
	max, wrote, err := ms.commitUserAdvert(ctx, user, now, nil)
	if err != nil || !wrote || max != 2 || userMax(t, db, user) != 2 {
		t.Fatalf("recompute max %d wrote %v err %v column %d", max, wrote, err, userMax(t, db, user))
	}
}
