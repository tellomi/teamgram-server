package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOpenPendingJobStoreUsesServiceMySQLConfig(t *testing.T) {
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	prevOpen := openPendingJobsDatabase
	prevPing := pingPendingJobsDB
	prevEnsure := ensurePendingJobsSchemaFn
	t.Cleanup(func() {
		setPendingJobConfigDSN("")
		openPendingJobsDatabase = prevOpen
		pingPendingJobsDB = prevPing
		ensurePendingJobsSchemaFn = prevEnsure
	})

	setPendingJobConfigDSN("")
	store := openPendingJobStore()
	if _, ok := store.(*memoryPendingJobs); !ok {
		t.Fatalf("empty ChannelMySQL: got %T, want memory store", store)
	}

	var gotDSN string
	openPendingJobsDatabase = func(driverName, dsn string) (*sql.DB, error) {
		gotDSN = dsn
		return sql.Open("mysql", dsn)
	}
	pingPendingJobsDB = func(*sql.DB) error { return nil }
	ensurePendingJobsSchemaFn = func(*sql.DB) error { return nil }

	const cfgDSN = "teamgram:x@tcp(127.0.0.1:3306)/teamgram"
	setPendingJobConfigDSN(cfgDSN)
	store = openPendingJobStore()
	if _, ok := store.(*mysqlPendingJobs); !ok {
		t.Fatalf("configured ChannelMySQL: got %T, want mysql store", store)
	}
	if gotDSN != cfgDSN {
		t.Fatal("store opened a different DSN than ChannelMySQL")
	}

	const envDSN = "env-override@tcp(127.0.0.1:3306)/teamgram"
	t.Setenv("TEAMGRAM_MYSQL_DSN", envDSN)
	_ = openPendingJobStore()
	if gotDSN != envDSN {
		t.Fatal("TEAMGRAM_MYSQL_DSN should override ChannelMySQL when already set")
	}
}

func TestOrphanScanRunsAgainstMySQLStore(t *testing.T) {
	prev := pendingJobs
	t.Cleanup(func() { pendingJobs = prev })

	pendingJobs = newMemoryPendingJobs()
	if jobs := defaultScanOrphanPendingJobs(time.Now(), time.Minute, 10); len(jobs) != 0 {
		t.Fatalf("memory store must not scan messages, got %d job(s)", len(jobs))
	}

	registerOrphanTestDriver()
	recordingReset()
	db, err := sql.Open("pending_orphan_test", "orphan")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	pendingJobs = &mysqlPendingJobs{db: db, fallback: newMemoryPendingJobs()}
	now := time.Unix(1_700_000_100, 0)
	jobs := defaultScanOrphanPendingJobs(now, time.Minute, 10)
	if len(jobs) != 1 {
		t.Fatalf("mysql store returned %d job(s), want 1", len(jobs))
	}
	if jobs[0].UserID != 7 || jobs[0].MsgID != 95 || jobs[0].PeerID != 8 {
		t.Fatalf("unexpected orphan job: %+v", jobs[0])
	}
	queries := recordedQueryCopy()
	var sawTables, sawOrphan bool
	for _, q := range queries {
		if strings.Contains(q, "information_schema") {
			sawTables = true
		}
		if strings.Contains(q, "webPagePending") && strings.Contains(q, "pending_webpage_jobs") {
			sawOrphan = true
		}
	}
	if !sawTables || !sawOrphan {
		t.Fatalf("mysql orphan scan did not run expected SQL (tables=%v orphan=%v)", sawTables, sawOrphan)
	}
}

type orphanTestDriver struct{}

type orphanTestConn struct{}

type sliceRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

var (
	orphanDriverOnce sync.Once
	recordingMu      sync.Mutex
	recordedQueries  []string
)

func registerOrphanTestDriver() {
	orphanDriverOnce.Do(func() {
		sql.Register("pending_orphan_test", orphanTestDriver{})
	})
}

func recordingReset() {
	recordingMu.Lock()
	recordedQueries = nil
	recordingMu.Unlock()
}

func recordedQueryCopy() []string {
	recordingMu.Lock()
	defer recordingMu.Unlock()
	out := make([]string, len(recordedQueries))
	copy(out, recordedQueries)
	return out
}

func recordQuery(q string) {
	recordingMu.Lock()
	recordedQueries = append(recordedQueries, q)
	recordingMu.Unlock()
}

func (orphanTestDriver) Open(string) (driver.Conn, error) {
	return orphanTestConn{}, nil
}

func (orphanTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("no prepare")
}
func (orphanTestConn) Close() error              { return nil }
func (orphanTestConn) Begin() (driver.Tx, error) { return nil, errors.New("no tx") }

func (orphanTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	recordQuery(query)
	if strings.Contains(query, "information_schema") {
		return &sliceRows{cols: []string{"table_name"}, data: [][]driver.Value{{"messages"}}}, nil
	}
	return &sliceRows{
		cols: []string{"user_id", "user_message_box_id", "peer_type", "peer_id", "date2"},
		data: [][]driver.Value{{int64(7), int64(95), int64(2), int64(8), int64(1700000000)}},
	}, nil
}

func (r *sliceRows) Columns() []string { return r.cols }
func (r *sliceRows) Close() error      { return nil }
func (r *sliceRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}
