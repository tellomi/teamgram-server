package mysqlpool

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"
)

// Apply sets the pool limits. It does not dial.
func Apply(db *sql.DB, s Settings) {
	if db == nil {
		return
	}
	db.SetMaxOpenConns(s.MaxOpen)
	db.SetMaxIdleConns(s.MaxIdle)
	db.SetConnMaxLifetime(s.MaxLifetime)
	db.SetConnMaxIdleTime(s.MaxIdleTime)
}

// WithAcquire returns a context that ends when ctx ends or, when ctx has no
// deadline, after the acquire timeout. Pool waits in database/sql stop when
// this context is cancelled.
func WithAcquire(ctx context.Context, s Settings) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	wait := s.AcquireWait
	if wait <= 0 {
		wait = defaultAcquireWait
	}
	return context.WithTimeout(ctx, wait)
}

type sharedDB struct {
	db *sql.DB
	n  int
}

var (
	sharedMu sync.Mutex
	shared   = map[string]*sharedDB{}
)

// closedDBHook runs at the start of a closed-pool probe, before any
// connection is taken. Tests use it to prove the probe does not hold
// sharedMu. Production leaves it nil.
var closedDBHook func()

// Shared returns the process-wide pool for dsn. A second call with the same
// dsn returns the same *sql.DB and keeps it open until every Discard has
// run. sql.Open does not dial; a bad server fails on the first query.
//
// The closed-pool probe takes a connection. It must not run while sharedMu
// is held: the goroutine that owns the last connection has to Discard or
// Shared to put it back, and those calls need the same mutex. Holding the
// mutex across Conn deadlocks that goroutine until the probe's own timeout.
func Shared(dsn string) (*sql.DB, error) {
	sharedMu.Lock()
	if ent, ok := shared[dsn]; ok {
		db := ent.db
		sharedMu.Unlock()
		if !closedDB(db) {
			sharedMu.Lock()
			if cur, ok := shared[dsn]; ok && cur.db == db {
				cur.n++
				sharedMu.Unlock()
				return db, nil
			}
			sharedMu.Unlock()
			return Shared(dsn)
		}
		sharedMu.Lock()
		cur, ok := shared[dsn]
		if !ok || cur.db != db {
			sharedMu.Unlock()
			return Shared(dsn)
		}
		delete(shared, dsn)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		sharedMu.Unlock()
		return nil, err
	}
	Apply(db, Resolve(ProcessName(), KindSQL))
	shared[dsn] = &sharedDB{db: db, n: 1}
	sharedMu.Unlock()
	return db, nil
}

// closedDB reports whether db was closed. A live pool is not dialed when an
// idle connection is available. A closed pool returns "sql: database is
// closed" without a dial. The caller must not hold sharedMu.
func closedDB(db *sql.DB) bool {
	if closedDBHook != nil {
		closedDBHook()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return strings.Contains(err.Error(), "database is closed")
	}
	_ = conn.Close()
	return false
}

// Discard releases one user of a shared pool. The *sql.DB is closed when
// the last user releases it. Callers must not call Close on a shared pool:
// that closes it for every other user. A pool this process did not share is
// closed immediately.
func Discard(db *sql.DB) error {
	if db == nil {
		return nil
	}
	sharedMu.Lock()
	for k, ent := range shared {
		if ent.db != db {
			continue
		}
		ent.n--
		if ent.n > 0 {
			sharedMu.Unlock()
			return nil
		}
		delete(shared, k)
		sharedMu.Unlock()
		return db.Close()
	}
	sharedMu.Unlock()
	return db.Close()
}

// ResetSharedForTest drops the map without closing. Tests use it so a later
// case does not see a closed pool.
func ResetSharedForTest() {
	sharedMu.Lock()
	shared = map[string]*sharedDB{}
	sharedMu.Unlock()
}

// Conn acquires one connection. The wait uses WithAcquire when ctx has no
// deadline. The caller closes the returned Conn.
func Conn(ctx context.Context, db *sql.DB, s Settings) (*sql.Conn, error) {
	ctx, cancel := WithAcquire(ctx, s)
	defer cancel()
	return db.Conn(ctx)
}

// idle time is part of Settings. Keep the import used when tests build
// a zero Settings and Conn fills the wait.
var _ = time.Second
