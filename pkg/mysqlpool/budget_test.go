package mysqlpool

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

func TestProductionSum(t *testing.T) {
	if got := SumOpen(); got != AppMax {
		t.Fatalf("steady-state max open = %d, want %d", got, AppMax)
	}
	if AppMax+OpsMax != ServerMax || ServerMax > 105 {
		t.Fatalf("server budget %d", ServerMax)
	}
	seen := map[string]bool{}
	for _, lim := range Production {
		if lim.MaxOpen < 1 {
			t.Fatalf("%s %s max open %d", lim.Process, lim.Kind, lim.MaxOpen)
		}
		if lim.MaxIdle < 0 || lim.MaxIdle > lim.MaxOpen {
			t.Fatalf("%s %s idle %d open %d", lim.Process, lim.Kind, lim.MaxIdle, lim.MaxOpen)
		}
		key := lim.Process + "/" + string(lim.Kind)
		if seen[key] {
			t.Fatalf("duplicate %s", key)
		}
		seen[key] = true
	}
	s := Resolve("biz", KindGoZero)
	if s.MaxLifetime != 30*time.Minute || s.MaxIdleTime != 5*time.Minute || s.AcquireWait != 5*time.Second {
		t.Fatalf("defaults %+v", s)
	}
}

func TestEnvOverrideAndInvalidIgnored(t *testing.T) {
	t.Setenv("TELLOMI_MYSQL_POOL", "biz")
	t.Setenv("TELLOMI_MYSQL_MAX_OPEN", "3")
	t.Setenv("TELLOMI_MYSQL_MAX_IDLE", "9")
	t.Setenv("TELLOMI_MYSQL_CONN_MAX_LIFETIME", "1m")
	t.Setenv("TELLOMI_MYSQL_ACQUIRE_TIMEOUT", "nope")
	s := Resolve(ProcessName(), KindGoZero)
	if s.MaxOpen != 3 || s.MaxIdle != 3 {
		t.Fatalf("open/idle %+v", s)
	}
	if s.MaxLifetime != time.Minute {
		t.Fatalf("lifetime %s", s.MaxLifetime)
	}
	if s.AcquireWait != 5*time.Second {
		t.Fatalf("bad acquire timeout was accepted: %s", s.AcquireWait)
	}
}

func TestUnknownProcessIsSmall(t *testing.T) {
	lim := Lookup("mediaretention", KindSQL)
	if lim.MaxOpen != unknownMaxOpen {
		t.Fatalf("tool cap %d", lim.MaxOpen)
	}
}

func TestSharedDedupesAndDiscard(t *testing.T) {
	ResetSharedForTest()
	t.Cleanup(ResetSharedForTest)
	t.Setenv("TELLOMI_MYSQL_POOL", "linkpreview")
	const dsn = "user:pass@tcp(127.0.0.1:1)/teamgram?timeout=50ms"
	db1, err := Shared(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db2, err := Shared(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if db1 != db2 {
		t.Fatal("same DSN opened two pools")
	}
	if got := db1.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("max open %d, want linkpreview sql 4", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err = db1.PingContext(ctx); err == nil {
		t.Fatal("ping of a closed port succeeded")
	}
	if got := db1.Stats().OpenConnections; got != 0 {
		t.Fatalf("failed init left %d connections", got)
	}
	// Two users. One release must leave the pool open.
	if err = Discard(db1); err != nil {
		t.Fatal(err)
	}
	db3, err := Shared(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if db3 != db1 {
		t.Fatal("pool was closed while another user still held it")
	}
	if err = Discard(db2); err != nil {
		t.Fatal(err)
	}
	if err = Discard(db3); err != nil {
		t.Fatal(err)
	}
	db4, err := Shared(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if db4 == db1 {
		t.Fatal("last discard did not drop the pool")
	}
	_ = Discard(db4)
}

func TestClosedSharedPoolIsReplaced(t *testing.T) {
	ResetSharedForTest()
	t.Cleanup(ResetSharedForTest)
	t.Setenv("TELLOMI_MYSQL_POOL", "dfs")
	const dsn = "user:pass@tcp(127.0.0.1:1)/teamgram?timeout=50ms"
	db1, err := Shared(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// A one-shot reader that calls Close instead of Discard used to leave
	// this pointer in the map. The next opener must not receive it.
	if err = db1.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := Shared(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if db2 == db1 {
		t.Fatal("closed pool was reused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err = db2.PingContext(ctx)
	if err != nil && strings.Contains(err.Error(), "database is closed") {
		t.Fatal(err)
	}
	_ = Discard(db2)
}

func TestWithAcquireCancelAndDeadline(t *testing.T) {
	s := Settings{AcquireWait: 5 * time.Second}
	ctx, cancel := WithAcquire(context.Background(), s)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second || time.Until(deadline) < 4*time.Second {
		t.Fatalf("deadline %v ok %v", deadline, ok)
	}
	parent, stop := context.WithTimeout(context.Background(), time.Hour)
	defer stop()
	child, stopChild := WithAcquire(parent, s)
	defer stopChild()
	got, _ := child.Deadline()
	want, _ := parent.Deadline()
	if !got.Equal(want) {
		t.Fatal("existing deadline was replaced")
	}
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	wrapped, stopWrap := WithAcquire(cancelled, s)
	defer stopWrap()
	if err := wrapped.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestSaturationRecovers(t *testing.T) {
	sql.Register("mysqlpool-fake", fakeDriver{})
	db, err := sql.Open("mysqlpool-fake", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	Apply(db, Settings{MaxOpen: 1, MaxIdle: 1, MaxLifetime: time.Hour, MaxIdleTime: time.Minute, AcquireWait: time.Second})
	held, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err = db.Conn(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("saturated acquire err %v", err)
	}
	cancelCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err = db.Conn(cancelCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquire err %v", err)
	}
	if err = held.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Conn(context.Background(), db, Settings{AcquireWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err = again.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInitFailure(t *testing.T) {
	sql.Register("mysqlpool-fail", failDriver{})
	db, err := sql.Open("mysqlpool-fail", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	Apply(db, Settings{MaxOpen: 2, MaxIdle: 1, MaxLifetime: time.Hour, MaxIdleTime: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = db.PingContext(ctx)
	if err == nil || !errors.Is(err, errDial) {
		t.Fatalf("err %v", err)
	}
	if got := db.Stats().OpenConnections; got != 0 {
		t.Fatalf("open connections %d", got)
	}
	if got := db.Stats().MaxOpenConnections; got != 2 {
		t.Fatalf("cap changed on failure: %d", got)
	}
}

func TestClampSQLXDoesNotPanic(t *testing.T) {
	t.Setenv("TELLOMI_MYSQL_POOL", "authsession")
	db := sqlx.NewMySQL(&sqlx.Config{DSN: "user:pass@tcp(127.0.0.1:1)/teamgram?timeout=200ms"})
	ClampSQLX(db)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := db.QueryRow(ctx, new(int), "SELECT 1")
	if err == nil {
		t.Fatal("query against a closed port succeeded")
	}
}

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return fakeConn{}, nil }

type fakeConn struct{}

func (fakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("nop") }
func (fakeConn) Close() error                        { return nil }
func (fakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("nop") }

var errDial = errors.New("dial failed")

type failDriver struct{}

func (failDriver) Open(string) (driver.Conn, error) { return nil, errDial }

// Keep the race detector honest if several tests touch Resolve.
var _ sync.Mutex
