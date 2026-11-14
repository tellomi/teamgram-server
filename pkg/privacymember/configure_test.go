package privacymember

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestConfigureUsesBFFMySQLWithoutEnv(t *testing.T) {
	resetPrivacyDB(t)
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	drv := &recordingDriver{}
	openDB = func(dsn string) (*sql.DB, error) {
		drv.mu.Lock()
		drv.opened = dsn
		drv.mu.Unlock()
		return sql.Open("privacy-config", dsn)
	}

	const configured = "bff-user:bff-secret@tcp(mysql:3306)/teamgram"
	Configure(configured)

	ctx := mtproto.WithPrivacyCache(context.Background())
	Prefetch(ctx, 5, []int64{8}, []int64{10})
	member, known := mtproto.Membership(ctx, 5, []int64{10})
	if !known || !member {
		t.Fatalf("chat membership member=%v known=%v", member, known)
	}
	blocked, ok := mtproto.OwnerBlockedViewer(ctx, 8)
	if !ok || !blocked {
		t.Fatalf("blocked owner blocked=%v known=%v", blocked, ok)
	}
	drv.mu.Lock()
	opened := drv.opened
	drv.mu.Unlock()
	if opened != configured {
		t.Fatalf("opened %q, want the ChannelMySQL value", opened)
	}
}

func TestConfigureEnvOverridesBFFMySQL(t *testing.T) {
	resetPrivacyDB(t)
	t.Setenv("TEAMGRAM_MYSQL_DSN", "env-dsn")
	var opened string
	openDB = func(dsn string) (*sql.DB, error) {
		opened = dsn
		return sql.Open("privacy-config", dsn)
	}
	Configure("config-dsn")
	if opened != "env-dsn" {
		t.Fatalf("opened %q", opened)
	}
}

func TestConfigureWarnsOnceWhenMySQLMissing(t *testing.T) {
	resetPrivacyDB(t)
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	var msgs []string
	logMissing = func(msg string) { msgs = append(msgs, msg) }
	Configure("")
	Configure("")
	if len(msgs) != 1 {
		t.Fatalf("warnings %d: %v", len(msgs), msgs)
	}
	if strings.Contains(msgs[0], "@") || strings.Contains(msgs[0], "password") || strings.Contains(msgs[0], "tcp(") {
		t.Fatalf("warning contains a DSN: %s", msgs[0])
	}
}

func TestConfigureOpenFailureDoesNotLogDSN(t *testing.T) {
	resetPrivacyDB(t)
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	openDB = func(string) (*sql.DB, error) {
		return nil, errOpen
	}
	var msgs []string
	logMissing = func(msg string) { msgs = append(msgs, msg) }
	Configure("user:secret-password@tcp(mysql:3306)/teamgram")
	if len(msgs) != 1 {
		t.Fatalf("warnings %v", msgs)
	}
	if strings.Contains(msgs[0], "secret-password") || strings.Contains(msgs[0], "tcp(") {
		t.Fatalf("warning contains a DSN: %s", msgs[0])
	}
}

var errOpen = io.EOF

func resetPrivacyDB(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		mu.Lock()
		if db != nil {
			_ = db.Close()
		}
		db = nil
		startupChosen = false
		missingWarned = false
		openDB = mysqltls.Open
		logMissing = func(msg string) { logx.Error(msg) }
		mu.Unlock()
	})
}

func init() {
	sql.Register("privacy-config", &recordingDriver{})
}

type recordingDriver struct {
	mu     sync.Mutex
	opened string
}

func (d *recordingDriver) Open(string) (driver.Conn, error) {
	return &recordingConn{}, nil
}

type recordingConn struct{}

func (c *recordingConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *recordingConn) Close() error                        { return nil }
func (c *recordingConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c *recordingConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	idx := 1
	if strings.Contains(query, "user_peer_blocks") {
		idx = 2
	}
	var id int64
	if idx < len(args) {
		switch v := args[idx].Value.(type) {
		case int64:
			id = v
		}
	}
	return &recordingRows{vals: []int64{id}}, nil
}

type recordingRows struct {
	vals []int64
	i    int
}

func (r *recordingRows) Columns() []string { return []string{"id"} }
func (r *recordingRows) Close() error      { return nil }
func (r *recordingRows) Next(dest []driver.Value) error {
	if r.i >= len(r.vals) {
		return io.EOF
	}
	dest[0] = r.vals[r.i]
	r.i++
	return nil
}
