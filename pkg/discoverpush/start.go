package discoverpush

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/discoverrel"
	"github.com/teamgram/teamgram-server/pkg/push"
	"github.com/zeromicro/go-zero/core/logx"
)

// Start listens when TELLOMI_DISCOVER_PUSH=1. A missing key or a database
// that cannot be opened is logged and does not stop sync. Chat pushes keep
// working. addr "off" does not listen.
func Start(svc *push.Service, mysqlDSN string) {
	if os.Getenv(EnvEnabled) != "1" {
		return
	}
	if svc == nil {
		logx.Error("discover push: push service is nil")
		return
	}
	key, err := decodeKey(os.Getenv(EnvKey))
	if err != nil {
		logx.Error("discover push: push key is missing or invalid")
		return
	}
	if strings.TrimSpace(mysqlDSN) == "" {
		logx.Error("discover push: mysql dsn is empty")
		return
	}
	subjects, err := discoverrel.OpenMySQL(mysqlDSN)
	if err != nil {
		logx.Error("discover push: mysql open failed")
		return
	}
	events, err := OpenEvents(mysqlDSN)
	if err != nil {
		logx.Error("discover push: event store open failed")
		return
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
		Handler:           Handler{Subjects: subjects, Events: events, Push: svc, Key: key},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("discover push listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("discover push http failed")
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
