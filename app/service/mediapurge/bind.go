package mediapurge

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	EnvEnabled  = "TELLOMI_MEDIA_PURGE_ENABLED"
	EnvDryRun   = "TELLOMI_MEDIA_PURGE_DRY_RUN"
	EnvGrace    = "TELLOMI_MEDIA_PURGE_GRACE"
	EnvInterval = "TELLOMI_MEDIA_PURGE_INTERVAL"
	EnvBatch    = "TELLOMI_MEDIA_PURGE_BATCH"
)

// Config is the background worker. Enqueue on delete does not look at
// Enabled: the queue fills either way. Enabled starts the loop. DryRun
// logs the objects and does not delete them. Grace is how long a queued
// id waits so an in-flight send or forward can land its message row.
type Config struct {
	Enabled  bool
	DryRun   bool
	Grace    time.Duration
	Interval time.Duration
	Batch    int
}

func DefaultConfig() Config {
	return Config{
		DryRun:   true,
		Grace:    24 * time.Hour,
		Interval: time.Minute,
		Batch:    100,
	}
}

// FromEnv reads TELLOMI_MEDIA_PURGE_*. An unset dry-run flag stays true.
// Grace accepts a Go duration ("24h", "1m") or a number of seconds.
func FromEnv() Config {
	c := DefaultConfig()
	c.Enabled = envBool(EnvEnabled, false)
	c.DryRun = envBool(EnvDryRun, true)
	if d, ok := envDuration(EnvGrace); ok && d >= 0 {
		c.Grace = d
	}
	if d, ok := envDuration(EnvInterval); ok && d > 0 {
		c.Interval = d
	}
	if n, ok := envInt(EnvBatch); ok && n > 0 {
		c.Batch = n
	}
	return c
}

func envBool(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(name string) (int, bool) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func envDuration(name string) (time.Duration, bool) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d, true
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return time.Duration(n) * time.Second, true
}

// Grace is the delay used when a delete enqueues an id.
func Grace() time.Duration {
	if d, ok := envDuration(EnvGrace); ok && d >= 0 {
		return d
	}
	return DefaultConfig().Grace
}

var (
	bindMu sync.Mutex
	bound  *Store
)

// Bind installs the process-wide store. A nil db clears it. The schema
// statements are applied immediately; a failure is logged and the store is
// still installed so a later call can retry.
func Bind(db *sql.DB) *Store {
	bindMu.Lock()
	defer bindMu.Unlock()
	if db == nil {
		bound = nil
		return nil
	}
	s := New(db)
	if err := s.Ensure(context.Background()); err != nil {
		log.Printf("media purge: schema: %v", err)
	}
	bound = s
	return s
}

// Default is the store installed by Bind. Nil means purge tracking is off
// and deletes still wipe message text.
func Default() *Store {
	bindMu.Lock()
	defer bindMu.Unlock()
	return bound
}

// Open connects and installs the store. An empty dsn leaves the store unset.
func Open(dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return Bind(db), nil
}
