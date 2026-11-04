package discoverprofile

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Start listens for the engine when TELLOMI_DISCOVER_PROFILES=1.
// A missing key, a bad key, or a database that cannot be opened is logged
// and does not stop bff. Login and messages.sendMessage do not call this.
func Start(mysqlDSN string) {
	if os.Getenv(EnvEnabled) != "1" {
		return
	}
	key, err := decodeKey(os.Getenv(EnvKey))
	if err != nil {
		logx.Error("discover profiles: profile key is missing or invalid")
		return
	}
	if strings.TrimSpace(mysqlDSN) == "" {
		logx.Error("discover profiles: mysql dsn is empty")
		return
	}
	db, err := OpenMySQL(mysqlDSN)
	if err != nil {
		logx.Errorf("discover profiles: mysql open failed")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = db.Ping(ctx)
	cancel()
	if err != nil {
		logx.Errorf("discover profiles: mysql ping failed")
	}
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
		logx.Infof("discover profiles listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("discover profiles http failed")
		}
	}()
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
