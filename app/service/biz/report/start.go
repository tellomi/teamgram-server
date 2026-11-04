package report

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/tellomiai"
	"github.com/zeromicro/go-zero/core/logx"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// EnvReview turns the model classifier on. 1 turns it on. 0 forces a
	// person. Empty follows TELLOMI_AI when that service is on.
	EnvReview = "TELLOMI_REPORT_REVIEW"
	// EnvSnapshot writes evidence objects when set to 1.
	EnvSnapshot = "TELLOMI_REPORT_SNAPSHOT"
	// EnvSnapshotBucket overrides the backups bucket for evidence objects.
	EnvSnapshotBucket = "TELLOMI_REPORT_SNAPSHOT_BUCKET"
	// EnvHTTP turns the human-review listener on. Only the value 1 counts.
	EnvHTTP = "TELLOMI_REPORT_HTTP"
	// EnvKey is standard base64 of at least 32 bytes.
	EnvKey = "TELLOMI_REPORT_KEY"
	// EnvAddr is the listen address. Empty means 127.0.0.1:8816.
	EnvAddr = "TELLOMI_REPORT_ADDR"
	// EnvLimit is reports per reporter per window. Empty means 20.
	EnvLimit = "TELLOMI_REPORT_LIMIT"
	// EnvWindow is the rate-limit window in seconds. Empty means 3600.
	EnvWindow = "TELLOMI_REPORT_WINDOW_SECONDS"
	// EnvETAHours is the expected review time told to the reporter. Empty means 24.
	EnvETAHours = "TELLOMI_REPORT_ETA_HOURS"

	defaultAddr = "127.0.0.1:8816"
)

var (
	regMu  sync.Mutex
	shared *Service
)

// Shared is the process-wide report service.
func Shared() *Service {
	regMu.Lock()
	defer regMu.Unlock()
	if shared == nil {
		shared = New(NewMemory())
	}
	return shared
}

// ResetShared drops process state. Tests call it.
func ResetShared() {
	regMu.Lock()
	shared = New(NewMemory())
	regMu.Unlock()
}

// UseDSN opens MySQL when dsn or TEAMGRAM_MYSQL_DSN is set and installs it
// as the shared service. An empty dsn keeps the current service.
func UseDSN(dsn string) error {
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	if strings.TrimSpace(dsn) == "" {
		return nil
	}
	svc, err := OpenMySQL(dsn)
	if err != nil {
		return err
	}
	regMu.Lock()
	shared = svc
	regMu.Unlock()
	return nil
}

// OpenMySQL connects, creates the reports tables, backfills story_reports,
// and returns a service. Review and snapshots stay off.
func OpenMySQL(dsn string) (*Service, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = ensureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := newMySQLStore(db)
	svc := New(store)
	svc.SetLangs(langDB{db: db})
	if n, berr := BackfillStoryReports(ctx, db, store); berr != nil {
		logx.Error("report: story backfill failed")
	} else if n > 0 {
		logx.Infof("report: backfilled %d story reports", n)
	}
	return svc, nil
}

// Start installs MySQL when dsn is set, applies env switches, and listens
// for human decisions when TELLOMI_REPORT_HTTP=1. A failure is logged and
// does not stop the process. Login is not involved.
func Start(dsn string, n Notifier) {
	if err := UseDSN(dsn); err != nil {
		logx.Error("report: mysql is unavailable; reports stay in process memory")
	}
	s := Shared()
	if n != nil {
		s.SetNotifier(n)
	}
	s.ApplyEnv()
	s.logReady()
	listen(s)
}

// logReady is the startup line operators should grep for. Receipts do not
// depend on the review or HTTP switches. A missing notifier means a report
// is stored and the reporter gets no @admin message.
func (s *Service) logReady() {
	if s == nil {
		return
	}
	reviewOn, reviewWhy := reviewPlan()
	http := planHTTP()
	notifier := "missing"
	if s.notifier != nil {
		notifier = "on"
	}
	review := "off (" + reviewWhy + ")"
	if reviewOn {
		review = "on (" + reviewWhy + ")"
	}
	httpState := "off (" + http.why + ")"
	if http.on {
		httpState = "on " + http.addr
		if loopback(http.addr) {
			httpState += " loopback"
		}
	}
	logx.Infof("report: ready store=%s notifier=%s review=%s http=%s", s.storeName(), notifier, review, httpState)
}

func (s *Service) storeName() string {
	if s == nil || s.store == nil {
		return "none"
	}
	if _, ok := s.store.(*mysqlStore); ok {
		return "mysql"
	}
	return "memory"
}

// ApplyEnv reads the report switches. The HTTP listener stays off unless
// TELLOMI_REPORT_HTTP=1. Review follows reviewPlan.
func (s *Service) ApplyEnv() {
	if s == nil {
		return
	}
	if n := envInt(EnvLimit, s.limit); n > 0 && n <= 1000 {
		s.limit = n
	}
	if n := envInt(EnvWindow, int(s.window)); n > 0 && n <= 7*24*3600 {
		s.window = int32(n)
	}
	if n := envInt(EnvETAHours, s.etaHours); n > 0 && n <= 24*30 {
		s.etaHours = n
	}
	if obj := openSnapshot(); obj != nil {
		s.SetObjects(obj)
	}
	if r := openReviewer(); r != nil {
		s.SetReviewer(r)
	}
}

// reviewPlan decides whether a new row is classified.
// TELLOMI_REPORT_REVIEW=1 turns it on. 0 forces a person.
// An empty value follows TELLOMI_AI when that service is actually on.
func reviewPlan() (on bool, why string) {
	switch strings.TrimSpace(os.Getenv(EnvReview)) {
	case "1":
		if strings.TrimSpace(os.Getenv(tellomiai.EnvAPIKey)) == "" {
			return false, "TELLOMI_REPORT_REVIEW=1 but TELLOMI_AI_API_KEY is empty"
		}
		return true, "TELLOMI_REPORT_REVIEW=1"
	case "0":
		return false, "TELLOMI_REPORT_REVIEW=0"
	case "":
		if tellomiai.Advertised() {
			return true, "follows TELLOMI_AI"
		}
		return false, "TELLOMI_REPORT_REVIEW is unset"
	default:
		return false, "TELLOMI_REPORT_REVIEW is not 1"
	}
}

func openReviewer() *Reviewer {
	on, _ := reviewPlan()
	if !on {
		if strings.TrimSpace(os.Getenv(EnvReview)) == "1" {
			logx.Error("report review is enabled but the model key is missing; reports wait for a person")
		}
		return nil
	}
	key := strings.TrimSpace(os.Getenv(tellomiai.EnvAPIKey))
	if key == "" {
		logx.Error("report review is enabled but the model key is missing; reports wait for a person")
		return nil
	}
	timeout := 12 * time.Second
	if raw := strings.TrimSpace(os.Getenv(tellomiai.EnvTimeoutSeconds)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 60 {
			timeout = time.Duration(n) * time.Second
		}
	}
	client := tellomiai.NewClient(
		os.Getenv(tellomiai.EnvBaseURL),
		key,
		os.Getenv(tellomiai.EnvModelPrimary),
		os.Getenv(tellomiai.EnvModelSmall),
		os.Getenv(tellomiai.EnvModelFallback),
		timeout,
	)
	return NewReviewer(client)
}

type httpPlan struct {
	on   bool
	addr string
	key  []byte
	why  string
}

func planHTTP() httpPlan {
	if os.Getenv(EnvHTTP) != "1" {
		return httpPlan{why: EnvHTTP + " is not 1"}
	}
	key, err := decodeKey(os.Getenv(EnvKey))
	if err != nil {
		return httpPlan{why: EnvKey + " is missing or invalid"}
	}
	addr := strings.TrimSpace(os.Getenv(EnvAddr))
	if addr == "" {
		addr = defaultAddr
	}
	if addr == "off" {
		return httpPlan{why: EnvAddr + "=off"}
	}
	return httpPlan{on: true, addr: addr, key: key}
}

func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

func listen(s *Service) {
	plan := planHTTP()
	if !plan.on {
		if os.Getenv(EnvHTTP) == "1" && strings.Contains(plan.why, EnvKey) {
			logx.Error("report review endpoint is enabled but the key is missing or invalid")
		}
		return
	}
	if loopback(plan.addr) {
		logx.Infof("report: http on %s is loopback; another container cannot connect. Set %s to an address content-engine can reach", plan.addr, EnvAddr)
	}
	key := plan.key
	addr := plan.addr
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler{Service: s, Key: key},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logx.Infof("report review listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Error("report review http failed")
		}
	}()
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrBadDecision
	}
	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		buf, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(buf) < 32 {
		return nil, ErrBadDecision
	}
	return buf, nil
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
