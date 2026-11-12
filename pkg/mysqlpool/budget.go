// Package mysqlpool is the MySQL connection budget for this process.
//
// MySQL max_connections is 151. The steady-state application cap is 96,
// which is under 70% of 151 (105.7). Ops keeps 9 (monitoring accounts are
// already limited to 3, plus migrate and an admin spare), so the server
// total stays at most 105.
//
// go-zero caches one *sql.DB per DSN inside a process and otherwise opens
// 64 connections. mysqltls.Open used to open another pool per caller.
// This package caps both. Pools opened with the same DSN in one process
// share one *sql.DB. A binary that is not in the table below is capped at
// 2, so a one-shot tool cannot take the steady-state budget. Do not run
// those tools while every server pool is full: their 2 connections come
// out of the ops spare.
//
// Steady-state max open connections (idle is never above max open):
//
//	idgen         sql      2   (seqfloor heal)
//	authsession   go-zero  8
//	dfs           sql      4   (media blobs)
//	media         go-zero  8
//	media         sql      4   (retention, purge, blobs)
//	biz           go-zero 20   (user, chat, dialog, message, updates, code)
//	biz           sql      4   (phone call binding)
//	msg           go-zero 12   (msg and inbox share one pool)
//	msg           sql      4   (ttl, chatbox, stories)
//	sync          go-zero  8
//	sync          sql      4   (push devices)
//	bff           go-zero  6   (folders, channels)
//	bff           sql      8   (feature stores share one pool)
//	linkpreview   sql      4
//	                     ---
//	                      96
//
// Every pool uses ConnMaxLifetime 30m, ConnMaxIdleTime 5m, and a 5s acquire
// timeout when the caller has no deadline. TELLOMI_MYSQL_MAX_OPEN,
// TELLOMI_MYSQL_MAX_IDLE, TELLOMI_MYSQL_CONN_MAX_LIFETIME,
// TELLOMI_MYSQL_CONN_MAX_IDLE_TIME, and TELLOMI_MYSQL_ACQUIRE_TIMEOUT
// override the current process. Raising max open on one process without
// lowering another can exceed 96.
package mysqlpool

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// AppMax is the steady-state sum of every server pool.
	AppMax = 96
	// OpsMax is reserved for monitoring, migrate, and an admin session.
	OpsMax = 9
	// ServerMax is AppMax + OpsMax. MySQL max_connections is 151.
	ServerMax = AppMax + OpsMax

	defaultLifetime    = 30 * time.Minute
	defaultIdleTime    = 5 * time.Minute
	defaultAcquireWait = 5 * time.Second
	unknownMaxOpen     = 2
)

// Kind selects which pool inside a process the limit applies to.
type Kind string

const (
	KindGoZero Kind = "go-zero"
	KindSQL    Kind = "sql"
)

// Limit is one pool's configured maximum.
type Limit struct {
	Process string
	Kind    Kind
	MaxOpen int
	MaxIdle int
}

// Production is the steady-state split. Tests fail if the sum exceeds AppMax.
var Production = []Limit{
	{Process: "idgen", Kind: KindSQL, MaxOpen: 2, MaxIdle: 1},
	{Process: "authsession", Kind: KindGoZero, MaxOpen: 8, MaxIdle: 2},
	{Process: "dfs", Kind: KindSQL, MaxOpen: 4, MaxIdle: 2},
	{Process: "media", Kind: KindGoZero, MaxOpen: 8, MaxIdle: 2},
	{Process: "media", Kind: KindSQL, MaxOpen: 4, MaxIdle: 2},
	{Process: "biz", Kind: KindGoZero, MaxOpen: 20, MaxIdle: 8},
	{Process: "biz", Kind: KindSQL, MaxOpen: 4, MaxIdle: 2},
	{Process: "msg", Kind: KindGoZero, MaxOpen: 12, MaxIdle: 4},
	{Process: "msg", Kind: KindSQL, MaxOpen: 4, MaxIdle: 2},
	{Process: "sync", Kind: KindGoZero, MaxOpen: 8, MaxIdle: 2},
	{Process: "sync", Kind: KindSQL, MaxOpen: 4, MaxIdle: 2},
	{Process: "bff", Kind: KindGoZero, MaxOpen: 6, MaxIdle: 2},
	{Process: "bff", Kind: KindSQL, MaxOpen: 8, MaxIdle: 2},
	{Process: "linkpreview", Kind: KindSQL, MaxOpen: 4, MaxIdle: 2},
}

// Settings is what database/sql is configured with.
type Settings struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration
	AcquireWait time.Duration
}

// ProcessName is the current server. TELLOMI_MYSQL_POOL overrides the
// executable name, which tests and one-shot wrappers use.
func ProcessName() string {
	if v := strings.TrimSpace(os.Getenv("TELLOMI_MYSQL_POOL")); v != "" {
		return v
	}
	base := filepath.Base(os.Args[0])
	return strings.TrimSuffix(base, ".exe")
}

// Lookup returns the steady-state limit for this process and kind.
// An unknown process gets unknownMaxOpen so it cannot open the go-zero
// default of 64.
func Lookup(process string, kind Kind) Limit {
	for _, lim := range Production {
		if lim.Process == process && lim.Kind == kind {
			return lim
		}
	}
	idle := 1
	if unknownMaxOpen < idle {
		idle = unknownMaxOpen
	}
	return Limit{Process: process, Kind: kind, MaxOpen: unknownMaxOpen, MaxIdle: idle}
}

// Resolve applies environment overrides on top of Lookup.
func Resolve(process string, kind Kind) Settings {
	lim := Lookup(process, kind)
	s := Settings{
		MaxOpen:     lim.MaxOpen,
		MaxIdle:     lim.MaxIdle,
		MaxLifetime: defaultLifetime,
		MaxIdleTime: defaultIdleTime,
		AcquireWait: defaultAcquireWait,
	}
	if n, ok := envPositive("TELLOMI_MYSQL_MAX_OPEN"); ok {
		s.MaxOpen = n
	}
	if n, ok := envPositive("TELLOMI_MYSQL_MAX_IDLE"); ok {
		s.MaxIdle = n
	}
	if s.MaxIdle > s.MaxOpen {
		s.MaxIdle = s.MaxOpen
	}
	if s.MaxIdle < 0 {
		s.MaxIdle = 0
	}
	if d, ok := envDuration("TELLOMI_MYSQL_CONN_MAX_LIFETIME"); ok {
		s.MaxLifetime = d
	}
	if d, ok := envDuration("TELLOMI_MYSQL_CONN_MAX_IDLE_TIME"); ok {
		s.MaxIdleTime = d
	}
	if d, ok := envDuration("TELLOMI_MYSQL_ACQUIRE_TIMEOUT"); ok {
		s.AcquireWait = d
	}
	if s.MaxOpen < 1 {
		s.MaxOpen = 1
	}
	if s.MaxLifetime <= 0 {
		s.MaxLifetime = defaultLifetime
	}
	if s.MaxIdleTime <= 0 {
		s.MaxIdleTime = defaultIdleTime
	}
	if s.AcquireWait <= 0 {
		s.AcquireWait = defaultAcquireWait
	}
	return s
}

func envPositive(key string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func envDuration(key string) (time.Duration, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

// SumOpen is the steady-state max-open total.
func SumOpen() int {
	n := 0
	for _, lim := range Production {
		n += lim.MaxOpen
	}
	return n
}
