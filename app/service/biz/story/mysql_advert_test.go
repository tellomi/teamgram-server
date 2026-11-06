package story

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Local throwaway server only. This test does not read TEAMGRAM_MYSQL_DSN.
// Fixture login for a local throwaway database. It is not a production
// credential. Each test creates its own schema and drops only that schema.
// The tests skip when this server is not listening.
const advertServerDSN = "storyadvert:storyadvert@tcp(127.0.0.1:3306)/?charset=utf8mb4&parseTime=true&multiStatements=true"

func openAdvertTest(t *testing.T) (*sql.DB, *mysqlStore, int32) {
	t.Helper()
	return openAdvertSchema(t, true)
}

func openBareAdvertTest(t *testing.T) (*sql.DB, *mysqlStore, int32) {
	t.Helper()
	return openAdvertSchema(t, false)
}

func openAdvertSchema(t *testing.T, advertTables bool) (*sql.DB, *mysqlStore, int32) {
	t.Helper()
	admin, err := sql.Open("mysql", advertServerDSN)
	if err != nil {
		t.Skipf("mysql open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Skipf("mysql not available: %v", err)
	}
	name := fmt.Sprintf("story_advert_%d_%d", os.Getpid(), advertSchemaSeq.Add(1))
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE `" + name + "`")
		_ = admin.Close()
	})
	dsn := "storyadvert:storyadvert@tcp(127.0.0.1:3306)/" + name + "?charset=utf8mb4&parseTime=true&multiStatements=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = ensureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var extra int
	if err = db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('story_advert_dirty', 'story_live_owner')`).Scan(&extra); err != nil {
		t.Fatal(err)
	}
	if extra != 0 {
		t.Fatalf("startup schema created %d advert tables", extra)
	}
	if _, err = db.ExecContext(ctx, `
CREATE TABLE users (
  id BIGINT NOT NULL PRIMARY KEY,
  stories_max_id INT NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatal(err)
	}
	if advertTables {
		if _, err = db.ExecContext(ctx, `
CREATE TABLE story_advert_dirty (
  user_id BIGINT NOT NULL,
  stories_max_id INT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (user_id)
)`); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `
CREATE TABLE story_live_owner (
  owner_id BIGINT NOT NULL,
  live_max INT NOT NULL,
  PRIMARY KEY (owner_id)
)`); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `ALTER TABLE users ADD INDEX users_stories_max (stories_max_id, id)`); err != nil {
			t.Fatal(err)
		}
	}
	return db, &mysqlStore{db: db}, int32(time.Now().Unix())
}

var advertUserSeq atomic.Int64
var advertSchemaSeq atomic.Int64

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

func TestMySQLDirtyCursorSkipsFailedHead(t *testing.T) {
	db, ms, now := openAdvertTest(t)
	ctx := context.Background()
	const extra = 300
	for i := 1; i <= extra; i++ {
		if _, err := db.Exec(`INSERT INTO story_advert_dirty (user_id, stories_max_id, updated_at) VALUES (?,?,?)`, i, 3, now); err != nil {
			t.Fatal(err)
		}
	}
	fail := true
	svc := New(ms)
	svc.SetClock(func() int32 { return now })
	svc.SetHooks(Hooks{SetUserMax: func(_ context.Context, id int64, _ int32) error {
		if fail && id == 1 {
			return errors.New("cache down")
		}
		return nil
	}})
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if dirtyCount(t, db, 1) != 1 {
		t.Fatal("failed head was cleared")
	}
	if n := dirtyCount(t, db, 0); n != 1+extra-256 {
		t.Fatalf("after first page %d dirty, want %d", n, 1+extra-256)
	}
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if dirtyCount(t, db, 1) != 1 || dirtyCount(t, db, 0) != 1 {
		t.Fatalf("tail not drained, head %d total %d", dirtyCount(t, db, 1), dirtyCount(t, db, 0))
	}
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if dirtyCount(t, db, 1) != 1 {
		t.Fatal("retry cleared the still-failing head")
	}
	fail = false
	svc.repairMySQLAdvertsLimit(ctx, 256)
	if dirtyCount(t, db, 0) != 0 {
		t.Fatalf("retry left %d", dirtyCount(t, db, 0))
	}
}

func TestMySQLUnmigratedDoesNotScanOrWrite(t *testing.T) {
	db, ms, now := openBareAdvertTest(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO users (id, stories_max_id) VALUES (1, 5)`); err != nil {
		t.Fatal(err)
	}
	insertStories(t, db, 1, 2000, now-10)
	var beforeStories int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stories`).Scan(&beforeStories); err != nil {
		t.Fatal(err)
	}
	_, _, err := ms.commitUserAdvert(ctx, 1, now, func(tx *sql.Tx) error {
		return upsertExec(ctx, tx, storyRow(1, 9000001, now+3600))
	})
	if !errors.Is(err, errAdvertSchema) {
		t.Fatalf("commit err %v", err)
	}
	var stories int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stories`).Scan(&stories); err != nil {
		t.Fatal(err)
	}
	if stories != beforeStories || userMax(t, db, 1) != 5 {
		t.Fatalf("unmigrated write kept story %d max %d", stories, userMax(t, db, 1))
	}
	db.SetMaxOpenConns(1)
	before := sessionStatus(t, db, "Handler_read_next")
	beforeRnd := sessionStatus(t, db, "Handler_read_rnd_next")
	svc := New(ms)
	svc.SetClock(func() int32 { return now })
	svc.repairMySQLAdvertsLimit(ctx, 256)
	next := sessionStatus(t, db, "Handler_read_next") - before
	rnd := sessionStatus(t, db, "Handler_read_rnd_next") - beforeRnd
	if next > 200 || rnd > 200 {
		t.Fatalf("unmigrated repair read next=%d rnd=%d", next, rnd)
	}
	if userMax(t, db, 1) != 5 {
		t.Fatalf("unmigrated repair changed max to %d", userMax(t, db, 1))
	}
}

func TestMySQLLiveOwnersPageDoesNotScanStories(t *testing.T) {
	db, ms, now := openAdvertTest(t)
	ctx := context.Background()
	const expired = 20000
	const active = 1000
	insertStories(t, db, 1, expired, now-10)
	for i := 1; i <= active; i++ {
		owner := int64(100000 + i)
		if _, err := db.Exec(`INSERT INTO users (id, stories_max_id) VALUES (?, 1)`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO stories (owner_kind, owner_id, story_id, sender_user_id, random_id, date2, expire_date, pinned, deleted, forwards, body) VALUES (1, ?, 1, ?, 0, ?, ?, 0, 0, 0, x'00')`, owner, owner, now, now+3600); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO story_live_owner (owner_id, live_max) VALUES (?, 1)`, owner); err != nil {
			t.Fatal(err)
		}
	}
	typ, key, table := explainPlan(t, db, `
SELECT owner_id, live_max FROM story_live_owner
WHERE owner_id > ?
ORDER BY owner_id
LIMIT ?`, 0, 256)
	t.Logf("MariaDB live owner plan table=%s type=%s key=%s", table, typ, key)
	if table != "story_live_owner" || typ != "range" || key != "PRIMARY" {
		t.Fatalf("live plan table=%s type=%s key=%s", table, typ, key)
	}
	db.SetMaxOpenConns(1)
	before := sessionStatus(t, db, "Handler_read_next")
	rows, err := ms.pageLiveOwners(ctx, 0, 256)
	if err != nil {
		t.Fatal(err)
	}
	got := sessionStatus(t, db, "Handler_read_next") - before
	t.Logf("MariaDB live owner page read %d", got)
	if len(rows) != 256 {
		t.Fatalf("page %d", len(rows))
	}
	if got > 800 {
		t.Fatalf("live page read %d index rows", got)
	}
	before = sessionStatus(t, db, "Handler_read_next")
	q, err := db.QueryContext(ctx, `
SELECT owner_id FROM stories
WHERE owner_kind=1 AND deleted=0 AND expire_date>? AND owner_id>?
GROUP BY owner_id
ORDER BY owner_id
LIMIT 256`, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for q.Next() {
		n++
	}
	q.Close()
	scanned := sessionStatus(t, db, "Handler_read_next") - before
	t.Logf("MariaDB grouped story page returned %d and read %d", n, scanned)
	if scanned <= 256 {
		t.Fatalf("grouped story page read %d, which is not more than the returned page", scanned)
	}
}

func dirtyCount(t *testing.T, db *sql.DB, id int64) int {
	t.Helper()
	var n int
	var err error
	if id == 0 {
		err = db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty`).Scan(&n)
	} else {
		err = db.QueryRow(`SELECT COUNT(*) FROM story_advert_dirty WHERE user_id=?`, id).Scan(&n)
	}
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func insertStories(t *testing.T, db *sql.DB, owner int64, n int, expire int32) {
	t.Helper()
	const batch = 500
	for at := 1; at <= n; {
		end := at + batch - 1
		if end > n {
			end = n
		}
		q := "INSERT INTO stories (owner_kind, owner_id, story_id, sender_user_id, random_id, date2, expire_date, pinned, deleted, forwards, body) VALUES "
		args := make([]any, 0, (end-at+1)*4)
		for id := at; id <= end; id++ {
			if id > at {
				q += ","
			}
			q += "(1,?,?,?,0,?,?,0,0,0,x'00')"
			args = append(args, owner, id, owner, expire-3600, expire)
		}
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
		at = end + 1
	}
}

func explainPlan(t *testing.T, db *sql.DB, query string, args ...any) (typ, key, table string) {
	t.Helper()
	typ, key = explainQuery(t, db, query, args...)
	rows, err := db.Query("EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
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
		table = got["table"]
		typ = got["type"]
		key = got["key"]
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return typ, key, table
}

func sessionStatus(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	if name != "Handler_read_next" && name != "Handler_read_rnd_next" {
		t.Fatalf("status %s", name)
	}
	var key, raw string
	q := "SHOW SESSION STATUS LIKE '" + name + "'"
	if err := db.QueryRowContext(context.Background(), q).Scan(&key, &raw); err != nil {
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
