package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	pendingWebpageMaxAge       = 60 * time.Second
	pendingSweepLimit          = 100
	pendingSweepEvery          = 15 * time.Second
	pendingJobMaxAttempts      = 10
	pendingJobGiveUpAfter      = time.Hour
	pendingJobsMaxOpenConns    = 2
	pendingJobsMaxIdleConns    = 1
	pendingJobsConnMaxLifetime = 30 * time.Minute
	pendingJobsCreateSQL       = "" +
		"CREATE TABLE IF NOT EXISTS `pending_webpage_jobs` (" +
		"  `user_id` bigint NOT NULL," +
		"  `msg_id` int NOT NULL," +
		"  `peer_type` int NOT NULL," +
		"  `peer_id` bigint NOT NULL," +
		"  `auth_key_id` bigint NOT NULL DEFAULT 0," +
		"  `created_unix` bigint NOT NULL," +
		"  `attempts` int NOT NULL DEFAULT 0," +
		"  `next_retry_unix` bigint NOT NULL DEFAULT 0," +
		"  PRIMARY KEY (`user_id`, `msg_id`)," +
		"  KEY `idx_pending_webpage_created` (`created_unix`)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
	pendingJobsInsertSQL = "INSERT INTO `pending_webpage_jobs` (`user_id`,`msg_id`,`peer_type`,`peer_id`,`auth_key_id`,`created_unix`,`attempts`,`next_retry_unix`) VALUES (?,?,?,?,?,?,0,0) ON DUPLICATE KEY UPDATE `peer_type`=VALUES(`peer_type`), `peer_id`=VALUES(`peer_id`), `auth_key_id`=VALUES(`auth_key_id`), `created_unix`=VALUES(`created_unix`), `attempts`=0, `next_retry_unix`=0"
	pendingJobsEnsureSQL = "INSERT IGNORE INTO `pending_webpage_jobs` (`user_id`,`msg_id`,`peer_type`,`peer_id`,`auth_key_id`,`created_unix`,`attempts`,`next_retry_unix`) VALUES (?,?,?,?,?,?,0,0)"
	pendingJobsDeleteSQL = "DELETE FROM `pending_webpage_jobs` WHERE `user_id`=? AND `msg_id`=?"
	pendingJobsDueSQL    = "SELECT `user_id`,`msg_id`,`peer_type`,`peer_id`,`auth_key_id`,`created_unix`,`attempts`,`next_retry_unix` FROM `pending_webpage_jobs` WHERE `created_unix`<=? AND `attempts`<? AND (`next_retry_unix`<=? OR `created_unix`<=?) ORDER BY `created_unix` ASC LIMIT ?"
	pendingJobsFailSQL   = "UPDATE `pending_webpage_jobs` SET `attempts`=?, `next_retry_unix`=? WHERE `user_id`=? AND `msg_id`=?"
	// Outgoing rows only (sender_user_id = user_id): one EditHide fans out
	// to the peer, so inbox copies are not scanned separately.
	pendingOrphanSelectSQL = "SELECT `user_id`,`user_message_box_id`,`peer_type`,`peer_id`,`date2` FROM `%s` WHERE `deleted`=0 AND `sender_user_id`=`user_id` AND `date2`<=? AND `date2`>=? AND `message_data` LIKE '%%webPagePending%%' AND NOT EXISTS (SELECT 1 FROM `pending_webpage_jobs` j WHERE j.`user_id`=`%s`.`user_id` AND j.`msg_id`=`%s`.`user_message_box_id`) ORDER BY `date2` ASC LIMIT ?"
	pendingOrphanTablesSQL = "SELECT `table_name` FROM information_schema.tables WHERE table_schema = DATABASE() AND (`table_name` = 'messages' OR `table_name` REGEXP '^messages_[0-9]+$')"
)

// PendingWebpageJob is one message still showing webPagePending.
type PendingWebpageJob struct {
	UserID    int64
	AuthKeyID int64
	PeerType  int32
	PeerID    int64
	MsgID     int32
	Created   time.Time
	Attempts  int
	NextRetry time.Time
}

// PendingJobStore is a bounded, indexed set of unfinished pending cards.
type PendingJobStore interface {
	Put(job PendingWebpageJob)
	Delete(userID int64, msgID int32)
	Due(now time.Time, olderThan time.Duration, limit int) []PendingWebpageJob
	// Fail records a retryable error: increments attempts and sets next_retry.
	// giveUp is true when attempts reach the cap or the job is older than 1 hour.
	Fail(job PendingWebpageJob, now time.Time) (updated PendingWebpageJob, giveUp bool)
	// Ensure inserts a job if missing and does not reset attempts on an existing row.
	Ensure(job PendingWebpageJob)
	// Tombstone keeps the row but excludes it from Due and the orphan scan,
	// so the same card is not requeued forever after the retry cap.
	Tombstone(job PendingWebpageJob)
}

type memoryPendingJobs struct {
	mu   sync.Mutex
	jobs map[[2]int64]PendingWebpageJob
}

func newMemoryPendingJobs() *memoryPendingJobs {
	return &memoryPendingJobs{jobs: map[[2]int64]PendingWebpageJob{}}
}

func (s *memoryPendingJobs) key(userID int64, msgID int32) [2]int64 {
	return [2]int64{userID, int64(msgID)}
}

func (s *memoryPendingJobs) Put(job PendingWebpageJob) {
	if job.UserID == 0 || job.MsgID == 0 {
		return
	}
	job.Attempts = 0
	job.NextRetry = time.Time{}
	s.mu.Lock()
	s.jobs[s.key(job.UserID, job.MsgID)] = job
	s.mu.Unlock()
}

func (s *memoryPendingJobs) Delete(userID int64, msgID int32) {
	s.mu.Lock()
	delete(s.jobs, s.key(userID, msgID))
	s.mu.Unlock()
}

func (s *memoryPendingJobs) Due(now time.Time, olderThan time.Duration, limit int) []PendingWebpageJob {
	if limit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PendingWebpageJob, 0)
	for _, job := range s.jobs {
		if !pendingJobIsDue(job, now, olderThan) {
			continue
		}
		out = append(out, job)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *memoryPendingJobs) Fail(job PendingWebpageJob, now time.Time) (PendingWebpageJob, bool) {
	updated, giveUp := recordPendingJobFailure(job, now)
	if job.UserID == 0 || job.MsgID == 0 {
		return updated, giveUp
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[s.key(job.UserID, job.MsgID)]; !ok {
		return updated, giveUp
	}
	s.jobs[s.key(job.UserID, job.MsgID)] = updated
	return updated, giveUp
}

func (s *memoryPendingJobs) Ensure(job PendingWebpageJob) {
	if job.UserID == 0 || job.MsgID == 0 {
		return
	}
	if job.Created.IsZero() {
		job.Created = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[s.key(job.UserID, job.MsgID)]; ok {
		return
	}
	job.Attempts = 0
	job.NextRetry = time.Time{}
	s.jobs[s.key(job.UserID, job.MsgID)] = job
}

func (s *memoryPendingJobs) Tombstone(job PendingWebpageJob) {
	if job.UserID == 0 || job.MsgID == 0 {
		return
	}
	job.Attempts = pendingJobMaxAttempts
	if job.Created.IsZero() {
		job.Created = time.Now()
	}
	s.mu.Lock()
	s.jobs[s.key(job.UserID, job.MsgID)] = job
	s.mu.Unlock()
}

type mysqlPendingJobs struct {
	db       *sql.DB
	fallback *memoryPendingJobs
}

func configurePendingJobsDB(db *sql.DB) {
	if db == nil {
		return
	}
	// The messages BFF Dao is still RPC-only; we open a dedicated pool
	// from the same ChannelMySQL DSN the other BFF DAOs use, capped at
	// 2 open / 1 idle so it stays inside the #308 budget
	// (add "messages bff pending_webpage_jobs 2/1" to that table).
	db.SetMaxOpenConns(pendingJobsMaxOpenConns)
	db.SetMaxIdleConns(pendingJobsMaxIdleConns)
	db.SetConnMaxLifetime(pendingJobsConnMaxLifetime)
}

func isMySQLDupColumn(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1060
}

func ensurePendingJobsSchema(db *sql.DB) error {
	if _, err := db.Exec(pendingJobsCreateSQL); err != nil {
		return err
	}
	// CREATE TABLE IF NOT EXISTS will not add columns to a table that was
	// created before attempts/next_retry_unix existed. ALTER is idempotent:
	// MySQL 8 has no ADD COLUMN IF NOT EXISTS, so we ignore error 1060.
	for _, q := range []string{
		"ALTER TABLE `pending_webpage_jobs` ADD COLUMN `attempts` int NOT NULL DEFAULT 0",
		"ALTER TABLE `pending_webpage_jobs` ADD COLUMN `next_retry_unix` bigint NOT NULL DEFAULT 0",
	} {
		if _, err := db.Exec(q); err != nil && !isMySQLDupColumn(err) {
			return err
		}
	}
	return nil
}

// pendingJobConfigDSN is the service MySQL DSN from messages Config.ChannelMySQL
// (filled by the all-in-one bff from its existing ChannelMySQL).
var pendingJobConfigDSN string

// openPendingJobsDatabase, pingPendingJobsDB, and ensurePendingJobsSchemaFn
// are replaced in tests so store selection can be asserted without a live server.
var openPendingJobsDatabase = sql.Open

var pingPendingJobsDB = func(db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}
	return db.Ping()
}

var ensurePendingJobsSchemaFn = ensurePendingJobsSchema

func setPendingJobConfigDSN(dsn string) {
	pendingJobConfigDSN = strings.TrimSpace(dsn)
}

// pendingJobDSN is the DSN the rest of the service already uses.
// TEAMGRAM_MYSQL_DSN, when already set, overrides it the same way other
// BFFs do. No new production env var is introduced.
func pendingJobDSN() string {
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		return env
	}
	return strings.TrimSpace(pendingJobConfigDSN)
}

func openPendingJobStore() PendingJobStore {
	mem := newMemoryPendingJobs()
	dsn := pendingJobDSN()
	if dsn == "" {
		logx.Errorf("pending webpage jobs: no service MySQL DSN (ChannelMySQL was empty); using in-memory store. Job persistence and the messages orphan scan are disabled. This is not safe for production.")
		return mem
	}
	db, err := openPendingJobsDatabase("mysql", dsn)
	if err != nil {
		logx.Errorf("pending webpage jobs: open mysql failed; falling back to in-memory store. Job persistence and the messages orphan scan are disabled: %v", err)
		return mem
	}
	configurePendingJobsDB(db)
	if err = pingPendingJobsDB(db); err != nil {
		logx.Errorf("pending webpage jobs: ping mysql failed; falling back to in-memory store. Job persistence and the messages orphan scan are disabled: %v", err)
		_ = db.Close()
		return mem
	}
	if err = ensurePendingJobsSchemaFn(db); err != nil {
		logx.Errorf("pending webpage jobs: schema failed; falling back to in-memory store. Job persistence and the messages orphan scan are disabled: %v", err)
		_ = db.Close()
		return mem
	}
	logx.Infof("pending webpage jobs: using MySQL store")
	return &mysqlPendingJobs{db: db, fallback: mem}
}

func (s *mysqlPendingJobs) Put(job PendingWebpageJob) {
	if s == nil || s.db == nil {
		if s != nil && s.fallback != nil {
			s.fallback.Put(job)
		}
		return
	}
	if job.Created.IsZero() {
		job.Created = time.Now()
	}
	_, err := s.db.Exec(pendingJobsInsertSQL, job.UserID, job.MsgID, job.PeerType, job.PeerID, job.AuthKeyID, job.Created.UTC().Unix())
	if err != nil {
		logx.Errorf("pending webpage jobs: put: %v", err)
		if s.fallback != nil {
			s.fallback.Put(job)
		}
	}
}

func (s *mysqlPendingJobs) Delete(userID int64, msgID int32) {
	if s == nil || s.db == nil {
		if s != nil && s.fallback != nil {
			s.fallback.Delete(userID, msgID)
		}
		return
	}
	if _, err := s.db.Exec(pendingJobsDeleteSQL, userID, msgID); err != nil {
		logx.Errorf("pending webpage jobs: delete: %v", err)
	}
	if s.fallback != nil {
		s.fallback.Delete(userID, msgID)
	}
}

func (s *mysqlPendingJobs) Due(now time.Time, olderThan time.Duration, limit int) []PendingWebpageJob {
	if s == nil || s.db == nil {
		if s != nil && s.fallback != nil {
			return s.fallback.Due(now, olderThan, limit)
		}
		return nil
	}
	if limit <= 0 {
		return nil
	}
	cutoff := now.Add(-olderThan).UTC().Unix()
	giveUpCutoff := now.Add(-pendingJobGiveUpAfter).UTC().Unix()
	rows, err := s.db.Query(pendingJobsDueSQL, cutoff, pendingJobMaxAttempts, now.UTC().Unix(), giveUpCutoff, limit)
	if err != nil {
		logx.Errorf("pending webpage jobs: due: %v", err)
		if s.fallback != nil {
			return s.fallback.Due(now, olderThan, limit)
		}
		return nil
	}
	defer rows.Close()
	var out []PendingWebpageJob
	for rows.Next() {
		var job PendingWebpageJob
		var created, nextRetry int64
		if err = rows.Scan(&job.UserID, &job.MsgID, &job.PeerType, &job.PeerID, &job.AuthKeyID, &created, &job.Attempts, &nextRetry); err != nil {
			logx.Errorf("pending webpage jobs: scan: %v", err)
			continue
		}
		job.Created = time.Unix(created, 0).UTC()
		if nextRetry > 0 {
			job.NextRetry = time.Unix(nextRetry, 0).UTC()
		}
		out = append(out, job)
	}
	return out
}

func (s *mysqlPendingJobs) Fail(job PendingWebpageJob, now time.Time) (PendingWebpageJob, bool) {
	updated, giveUp := recordPendingJobFailure(job, now)
	if s == nil || s.db == nil {
		if s != nil && s.fallback != nil {
			return s.fallback.Fail(job, now)
		}
		return updated, giveUp
	}
	_, err := s.db.Exec(pendingJobsFailSQL, updated.Attempts, updated.NextRetry.UTC().Unix(), updated.UserID, updated.MsgID)
	if err != nil {
		logx.Errorf("pending webpage jobs: fail: %v", err)
		if s.fallback != nil {
			return s.fallback.Fail(job, now)
		}
	}
	return updated, giveUp
}

func (s *mysqlPendingJobs) Ensure(job PendingWebpageJob) {
	if s == nil || s.db == nil {
		if s != nil && s.fallback != nil {
			s.fallback.Ensure(job)
		}
		return
	}
	if job.UserID == 0 || job.MsgID == 0 {
		return
	}
	if job.Created.IsZero() {
		job.Created = time.Now()
	}
	_, err := s.db.Exec(pendingJobsEnsureSQL, job.UserID, job.MsgID, job.PeerType, job.PeerID, job.AuthKeyID, job.Created.UTC().Unix())
	if err != nil {
		logx.Errorf("pending webpage jobs: ensure: %v", err)
		if s.fallback != nil {
			s.fallback.Ensure(job)
		}
	}
}

func (s *mysqlPendingJobs) Tombstone(job PendingWebpageJob) {
	if s == nil || s.db == nil {
		if s != nil && s.fallback != nil {
			s.fallback.Tombstone(job)
		}
		return
	}
	if job.UserID == 0 || job.MsgID == 0 {
		return
	}
	_, err := s.db.Exec(pendingJobsFailSQL, pendingJobMaxAttempts, time.Now().Add(pendingOrphanLookback).UTC().Unix(), job.UserID, job.MsgID)
	if err != nil {
		logx.Errorf("pending webpage jobs: tombstone: %v", err)
	}
	if s.fallback != nil {
		s.fallback.Tombstone(job)
	}
}

func pendingJobIsDue(job PendingWebpageJob, now time.Time, olderThan time.Duration) bool {
	if job.Attempts >= pendingJobMaxAttempts {
		return false
	}
	if pendingJobShouldGiveUp(job, now) {
		return true
	}
	if job.Created.After(now.Add(-olderThan)) {
		return false
	}
	return !job.NextRetry.After(now)
}

func pendingJobShouldGiveUp(job PendingWebpageJob, now time.Time) bool {
	if job.Attempts >= pendingJobMaxAttempts {
		return true
	}
	if !job.Created.IsZero() && !now.Before(job.Created.Add(pendingJobGiveUpAfter)) {
		return true
	}
	return false
}

func pendingBackoffUntil(now time.Time, attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}
	shift := attempts - 1
	const maxShift = 5 // 15s, 30s, 60s, 2m, 4m, 8m; 1h give-up still wins
	if shift > maxShift {
		shift = maxShift
	}
	return now.Add(pendingSweepEvery << shift)
}

func recordPendingJobFailure(job PendingWebpageJob, now time.Time) (PendingWebpageJob, bool) {
	job.Attempts++
	job.NextRetry = pendingBackoffUntil(now, job.Attempts)
	return job, pendingJobShouldGiveUp(job, now)
}

func pendingJobGiveUpReason(job PendingWebpageJob) string {
	if job.Attempts >= pendingJobMaxAttempts {
		return "max_attempts"
	}
	return "max_age"
}

// pendingJobs is the process-wide store. Tests replace it.
var pendingJobs PendingJobStore = newMemoryPendingJobs()

// pendingJobGiveUpLog is the warning emitted when a job is dropped. Tests replace it.
var pendingJobGiveUpLog = logPendingJobGiveUp

func logPendingJobGiveUp(job PendingWebpageJob, now time.Time, reason string) {
	logx.Errorf("pending webpage job given up (%s): user_id=%d msg_id=%d attempts=%d age=%s", reason, job.UserID, job.MsgID, job.Attempts, now.Sub(job.Created))
}

func initPendingJobStore() {
	pendingJobs = openPendingJobStore()
}

func trackPendingJob(job PendingWebpageJob) {
	if pendingJobs == nil {
		return
	}
	if job.Created.IsZero() {
		job.Created = time.Now()
	}
	pendingJobs.Put(job)
}

func untrackPendingJob(userID int64, msgID int32) {
	if pendingJobs != nil {
		pendingJobs.Delete(userID, msgID)
	}
}

func duePendingJobs(now time.Time) []PendingWebpageJob {
	if pendingJobs == nil {
		return nil
	}
	return pendingJobs.Due(now, pendingWebpageMaxAge, pendingSweepLimit)
}

func failPendingJob(job PendingWebpageJob, now time.Time) (PendingWebpageJob, bool) {
	if pendingJobs == nil {
		return recordPendingJobFailure(job, now)
	}
	return pendingJobs.Fail(job, now)
}

// scanOrphanPendingJobs finds webPagePending cards that have no job row
// (for example after a canceled Get was mistaken for Gone). Tests replace it.
var scanOrphanPendingJobs = defaultScanOrphanPendingJobs

func orphanPendingJobs(now time.Time) []PendingWebpageJob {
	if scanOrphanPendingJobs == nil {
		return nil
	}
	return scanOrphanPendingJobs(now, pendingWebpageMaxAge, pendingSweepLimit)
}

func defaultScanOrphanPendingJobs(now time.Time, olderThan time.Duration, limit int) []PendingWebpageJob {
	s, ok := pendingJobs.(*mysqlPendingJobs)
	if !ok || s == nil {
		return nil
	}
	return s.Orphans(now, olderThan, limit)
}

func (s *mysqlPendingJobs) Orphans(now time.Time, olderThan time.Duration, limit int) []PendingWebpageJob {
	if s == nil || s.db == nil || limit <= 0 {
		return nil
	}
	if olderThan <= 0 {
		olderThan = pendingWebpageMaxAge
	}
	ctx, cancel := context.WithTimeout(context.Background(), pendingCommitTimeout)
	defer cancel()
	tables, err := pendingMessageTables(ctx, s.db)
	if err != nil {
		logx.Errorf("pending webpage jobs: orphan tables: %v", err)
		tables = []string{"messages"}
	}
	newest := now.Add(-olderThan).UTC().Unix()
	oldest := now.Add(-pendingOrphanLookback).UTC().Unix()
	out := make([]PendingWebpageJob, 0)
	remain := limit
	for _, table := range tables {
		if remain <= 0 {
			break
		}
		if !pendingMessageTableOK(table) {
			continue
		}
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf(pendingOrphanSelectSQL, table, table, table), newest, oldest, remain)
		if err != nil {
			logx.Errorf("pending webpage jobs: orphan scan %s: %v", table, err)
			continue
		}
		for rows.Next() {
			var job PendingWebpageJob
			var created int64
			if err := rows.Scan(&job.UserID, &job.MsgID, &job.PeerType, &job.PeerID, &created); err != nil {
				logx.Errorf("pending webpage jobs: orphan scan: %v", err)
				continue
			}
			job.Created = time.Unix(created, 0).UTC()
			out = append(out, job)
			remain--
		}
		_ = rows.Close()
	}
	return out
}

func pendingMessageTableOK(name string) bool {
	if name == "messages" {
		return true
	}
	if len(name) < 10 || name[:9] != "messages_" {
		return false
	}
	for _, r := range name[9:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func pendingMessageTables(ctx context.Context, db *sql.DB) ([]string, error) {
	if db == nil {
		return []string{"messages"}, nil
	}
	rows, err := db.QueryContext(ctx, pendingOrphanTablesSQL)
	if err != nil {
		return []string{"messages"}, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return tables, err
		}
		if name != "" {
			tables = append(tables, name)
		}
	}
	if len(tables) == 0 {
		return []string{"messages"}, nil
	}
	return tables, nil
}

func ensurePendingJob(job PendingWebpageJob) {
	if pendingJobs == nil {
		return
	}
	if job.Created.IsZero() {
		job.Created = time.Now()
	}
	pendingJobs.Ensure(job)
}

func tombstonePendingJob(job PendingWebpageJob) {
	if pendingJobs == nil {
		return
	}
	pendingJobs.Tombstone(job)
}

func pendingJobIsTombstone(job PendingWebpageJob, now time.Time) bool {
	_ = now
	return job.Attempts >= pendingJobMaxAttempts
}
