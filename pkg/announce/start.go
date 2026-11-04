package announce

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Enabled reports whether the operator asked for the listener.
func Enabled() bool { return os.Getenv(EnvEnabled) == "1" }

// Advertised is the help.getAppConfig bit. A missing or short key stays off.
func Advertised() bool {
	if !Enabled() {
		return false
	}
	_, err := decodeKey(os.Getenv(EnvKey))
	return err == nil
}

// Start listens when the flag and key are set. A missing key or database
// is logged and does not stop bff. Login is not involved.
func Start(dsn string, users Users, channel Channels) {
	if !Enabled() {
		return
	}
	key, err := decodeKey(os.Getenv(EnvKey))
	if err != nil {
		logx.Error("announce: key is missing or invalid; listener is off")
		return
	}
	if strings.TrimSpace(dsn) == "" || users == nil {
		logx.Error("announce: mysql or sender is missing; listener is off")
		return
	}
	store, err := OpenMySQL(dsn)
	if err != nil {
		logx.Error("announce: mysql is unavailable; listener is off")
		return
	}
	addr := strings.TrimSpace(os.Getenv(EnvAddr))
	if addr == "" {
		addr = defaultAddr
	}
	if addr == "off" {
		return
	}
	svc := &Service{Dir: store, Claims: store, Users: users, Channel: channel}
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler{Service: svc, Key: key},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("announce listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Error("announce: http failed")
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
