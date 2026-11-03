package discoverrel

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	gate sync.RWMutex
	dir  Directory
	on   bool
)

// Start listens for the engine when TELLOMI_DISCOVER_RELATIONS=1.
// A missing key, a bad key, or a database that cannot be opened is logged
// and does not stop bff. addr "off" records subjects but does not listen.
func Start(mysqlDSN string) {
	if os.Getenv(EnvEnabled) != "1" {
		return
	}
	key, err := decodeKey(os.Getenv(EnvKey))
	if err != nil {
		logx.Error("discover relations: relation key is missing or invalid")
		return
	}
	if strings.TrimSpace(mysqlDSN) == "" {
		logx.Error("discover relations: mysql dsn is empty")
		return
	}
	db, err := OpenMySQL(mysqlDSN)
	if err != nil {
		logx.Errorf("discover relations: mysql open failed")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = db.Ping(ctx)
	cancel()
	if err != nil {
		logx.Errorf("discover relations: mysql ping failed")
	}
	setDirectory(db)
	addr := os.Getenv(EnvAddr)
	if addr == "" {
		addr = defaultAddr
	}
	if addr == "off" {
		return
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler{Dir: db, Key: key},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("discover relations listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("discover relations http failed")
		}
	}()
}

// Remember stores sub for userID when the relation API is on. It does not
// fail credential issuing. sub is 16 bytes and is not logged.
func Remember(ctx context.Context, userID int64, sub []byte) error {
	gate.RLock()
	active, d := on, dir
	gate.RUnlock()
	if !active || d == nil {
		return nil
	}
	return d.Save(ctx, userID, sub)
}

// UseDirectory installs a directory and marks the feature on. Tests use it.
func UseDirectory(d Directory) {
	setDirectory(d)
}

func setDirectory(d Directory) {
	gate.Lock()
	dir = d
	on = d != nil
	gate.Unlock()
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errBad
	}
	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		buf, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(buf) < 32 {
		return nil, errBad
	}
	return buf, nil
}
