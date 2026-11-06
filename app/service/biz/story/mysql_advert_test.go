package story

import (
	"context"
	"database/sql"
	"errors"
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
	if _, err = db.ExecContext(ctx, `ALTER TABLE users ADD INDEX IF NOT EXISTS users_stories_max (stories_max_id, id)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `ALTER TABLE stories ADD INDEX IF NOT EXISTS stories_owner_expire (owner_kind, deleted, expire_date, owner_id, story_id)`); err != nil {
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

func TestMySQLAdvertPlansAreBounded(t *testing.T) {
	db, ms, now := openAdvertTest(t)
	ctx := context.Background()
	const zeros = 2000
	const stale = 300
	base := int64(700_000_000)
	if _, err := db.Exec(`DELETE FROM users WHERE id BETWEEN ? AND ?`, base, base+zeros+stale); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM users WHERE id BETWEEN ? AND ?`, base, base+zeros+stale)
	})
	for i := 0; i < zeros; i++ {
		if _, err := db.Exec(`INSERT INTO users (id, stories_max_id) VALUES (?, 0)`, base+int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < stale; i++ {
		if _, err := db.Exec(`INSERT INTO users (id, stories_max_id) VALUES (?, 5)`, base+zeros+int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	typ, key := explainQuery(t, db, `
SELECT id, stories_max_id FROM users
WHERE stories_max_id > ? OR (stories_max_id = ? AND id > ?)
ORDER BY stories_max_id, id
LIMIT ?`, 0, 0, int64(^uint64(0)>>1), 256)
	t.Logf("MariaDB stale advert plan type=%s key=%s", typ, key)
	if typ == "ALL" || key != "users_stories_max" {
		t.Fatalf("stale plan type=%s key=%s, want a range on users_stories_max", typ, key)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	before := handlerCount(t, conn, "Handler_read_rnd_next")
	qrows, err := conn.QueryContext(ctx, `
SELECT id, stories_max_id FROM users
WHERE stories_max_id > ? OR (stories_max_id = ? AND id > ?)
ORDER BY stories_max_id, id
LIMIT ?`, 0, 0, int64(^uint64(0)>>1), 256)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for qrows.Next() {
		n++
	}
	qrows.Close()
	after := handlerCount(t, conn, "Handler_read_rnd_next")
	if n != 256 {
		t.Fatalf("page %d", n)
	}
	// A table scan of the zero rows would move this counter by about 2000.
	if after-before > 50 {
		t.Fatalf("page scanned %d table rows; zeros=%d", after-before, zeros)
	}

	svc := New(ms)
	svc.SetClock(func() int32 { return now })
	svc.repairMySQLAdvertsLimit(ctx, 256)
	var left int
	if err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id BETWEEN ? AND ? AND stories_max_id > 0`, base, base+zeros+stale).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != stale-256 {
		t.Fatalf("after first page %d non-zero, want %d", left, stale-256)
	}
	var zerosMoved int
	if err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id >= ? AND id < ? AND stories_max_id <> 0`, base, base+zeros).Scan(&zerosMoved); err != nil {
		t.Fatal(err)
	}
	if zerosMoved != 0 {
		t.Fatalf("zero advertisements rewritten: %d", zerosMoved)
	}
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id BETWEEN ? AND ? AND stories_max_id > 0`, base, base+zeros+stale).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("second page left %d", left)
	}
}

func TestMySQLDirtyCacheRetriesAfterCrash(t *testing.T) {
	db, ms, now := openAdvertTest(t)
	ctx := context.Background()
	user := advertUser(t, db)
	cache := map[int64]int32{user: 1}
	var calls int
	svc := New(ms)
	svc.SetClock(func() int32 { return now })
	svc.SetHooks(Hooks{SetUserMax: func(_ context.Context, id int64, max int32) error {
		calls++
		if calls == 1 {
			return errors.New("cache down")
		}
		cache[id] = max
		return nil
	}})
	if _, _, err := ms.commitUserAdvert(ctx, user, now, func(tx *sql.Tx) error {
		return upsertExec(ctx, tx, storyRow(user, 4, now+3600))
	}); err != nil {
		t.Fatal(err)
	}
	if cache[user] != 1 || userMax(t, db, user) != 4 {
		t.Fatalf("crash window cache %d sql %d", cache[user], userMax(t, db, user))
	}
	var dirty int
	if err := db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id=?`, user).Scan(&dirty); err != nil || dirty != 1 {
		t.Fatalf("dirty %d err %v", dirty, err)
	}
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if cache[user] != 1 {
		t.Fatalf("failed delete changed cache to %d", cache[user])
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id=?`, user).Scan(&dirty); err != nil || dirty != 1 {
		t.Fatalf("dirty after failed delete %d err %v", dirty, err)
	}
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if cache[user] != 4 {
		t.Fatalf("cache %d, want 4", cache[user])
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id=?`, user).Scan(&dirty); err != nil || dirty != 0 {
		t.Fatalf("dirty left %d err %v", dirty, err)
	}

	const extra = 300
	for i := 0; i < extra; i++ {
		id := int64(810_000_000 + i)
		if _, err := db.Exec(`INSERT INTO users (id, stories_max_id) VALUES (?, 0)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO story_advert_dirty (user_id, stories_max_id, updated_at) VALUES (?,?,?)`, id, 3, now); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM story_advert_dirty WHERE user_id BETWEEN 810000000 AND 810000300`)
		_, _ = db.Exec(`DELETE FROM users WHERE id BETWEEN 810000000 AND 810000300`)
	})
	svc.SetHooks(Hooks{SetUserMax: func(context.Context, int64, int32) error { return nil }})
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if err := db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id BETWEEN 810000000 AND 810000300`).Scan(&dirty); err != nil {
		t.Fatal(err)
	}
	if dirty != extra-256 {
		t.Fatalf("dirty left %d, want %d", dirty, extra-256)
	}
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if err := db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id BETWEEN 810000000 AND 810000300`).Scan(&dirty); err != nil {
		t.Fatal(err)
	}
	if dirty != 0 {
		t.Fatalf("second drain left %d", dirty)
	}
}

func explainQuery(t *testing.T, db *sql.DB, query string, args ...any) (typ, key string) {
	t.Helper()
	rows, err := db.Query("EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err = rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for i, col := range cols {
			got[col] = vals[i].String
		}
		if got["table"] == "users" || typ == "" {
			typ = got["type"]
			key = got["key"]
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return typ, key
}

func handlerCount(t *testing.T, conn *sql.Conn, name string) int {
	t.Helper()
	var key, raw string
	if name != "Handler_read_rnd_next" {
		t.Fatalf("status %s", name)
	}
	if err := conn.QueryRowContext(context.Background(), `SHOW SESSION STATUS LIKE 'Handler_read_rnd_next'`).Scan(&key, &raw); err != nil {
		t.Fatal(err)
	}
	val := 0
	for _, c := range raw {
		if c < '0' || c > '9' {
			t.Fatalf("status %s=%q", name, raw)
		}
		val = val*10 + int(c-'0')
	}
	return val
}
