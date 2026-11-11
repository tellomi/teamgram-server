package crashreport

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/redact"
	"github.com/zeromicro/go-zero/core/logx"
)

// Service stores reports and answers the query API.
type Service struct {
	db      *sql.DB
	objects Objects
	// docs and saveRow override MySQL in tests.
	docs    func(context.Context, int64) (docSource, error)
	saveRow func(context.Context, Row) error
}

// New is the collector the CLI uses. objects may be nil; list then omits download URLs.
func New(db *sql.DB, objects Objects) *Service {
	return &Service{db: db, objects: objects}
}

var (
	mu   sync.RWMutex
	live *Service
)

func current() *Service {
	mu.RLock()
	defer mu.RUnlock()
	return live
}

// Use installs s as the process collector. Tests pass nil to clear it.
func Use(s *Service) {
	mu.Lock()
	live = s
	mu.Unlock()
}

// Start ensures the bot, installs the collector, and listens when
// TELLOMI_CRASH_REPORT_TOKEN is set. A missing token leaves the listener off.
// A conflict with an existing user is logged and does not stop bff.
func Start(dsn string) {
	db, err := OpenMySQL(dsn)
	if err != nil || db == nil {
		logx.Error("crash report: mysql is unavailable; collector is off")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = Ensure(ctx, db); err != nil {
		mysqlpool.Discard(db)
		if errors.Is(err, ErrConflict) {
			logx.Error("crash report: bot id or username belongs to a different account; collector is off")
		} else {
			logx.Error("crash report: ensure failed; collector is off")
		}
		return
	}
	objs, err := OpenObjects()
	if err != nil {
		mysqlpool.Discard(db)
		logx.Error("crash report: object storage is unavailable; collector is off")
		return
	}
	svc := &Service{db: db, objects: objs}
	Use(svc)
	var dbName string
	if err = db.QueryRowContext(ctx, `SELECT IFNULL(username,'') FROM users WHERE id = ?`, AccountID).Scan(&dbName); err != nil {
		logx.Error("crash report: bot username was not read; publishing TellomiBot")
		dbName = DefaultUsername
	}
	UseDBUsername(dbName)
	token := strings.TrimSpace(os.Getenv(EnvToken))
	if token == "" {
		logx.Info("crash report: query listener is off")
		return
	}
	addr := strings.TrimSpace(os.Getenv(EnvAddr))
	if addr == "" {
		addr = defaultAddr
	}
	if !AddrAllowed(addr) {
		logx.Error("crash report: listen address is not loopback; listener is off")
		return
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler{Service: svc, Token: token},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("crash report listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("crash report: http failed: %s", listenErrText(err))
		}
	}()
}

// listenErrText is the listen failure written to the log. A password in a
// DSN-shaped error is masked. The admin token is not part of this error.
func listenErrText(err error) string {
	if err == nil {
		return ""
	}
	return redact.DSN(err.Error())
}
