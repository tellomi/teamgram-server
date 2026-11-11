package core

import (
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"

	_ "github.com/go-sql-driver/mysql"
)

const (
	pendingWebpageMaxAge = 60 * time.Second
	pendingSweepLimit    = 100
	pendingSweepEvery    = 15 * time.Second
	pendingJobsCreateSQL = "" +
		"CREATE TABLE IF NOT EXISTS `pending_webpage_jobs` (" +
		"  `user_id` bigint NOT NULL," +
		"  `msg_id` int NOT NULL," +
		"  `peer_type` int NOT NULL," +
		"  `peer_id` bigint NOT NULL," +
		"  `auth_key_id` bigint NOT NULL DEFAULT 0," +
		"  `created_unix` bigint NOT NULL," +
		"  PRIMARY KEY (`user_id`, `msg_id`)," +
		"  KEY `idx_pending_webpage_created` (`created_unix`)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
	pendingJobsInsertSQL = "INSERT INTO `pending_webpage_jobs` (`user_id`,`msg_id`,`peer_type`,`peer_id`,`auth_key_id`,`created_unix`) VALUES (?,?,?,?,?,?) ON DUPLICATE KEY UPDATE `peer_type`=VALUES(`peer_type`), `peer_id`=VALUES(`peer_id`), `auth_key_id`=VALUES(`auth_key_id`), `created_unix`=VALUES(`created_unix`)"
	pendingJobsDeleteSQL = "DELETE FROM `pending_webpage_jobs` WHERE `user_id`=? AND `msg_id`=?"
	pendingJobsDueSQL    = "SELECT `user_id`,`msg_id`,`peer_type`,`peer_id`,`auth_key_id`,`created_unix` FROM `pending_webpage_jobs` WHERE `created_unix`<=? ORDER BY `created_unix` ASC LIMIT ?"
)

// PendingWebpageJob is one message still showing webPagePending.
type PendingWebpageJob struct {
	UserID    int64
	AuthKeyID int64
	PeerType  int32
	PeerID    int64
	MsgID     int32
	Created   time.Time
}

// PendingJobStore is a bounded, indexed set of unfinished pending cards.
type PendingJobStore interface {
	Put(job PendingWebpageJob)
	Delete(userID int64, msgID int32)
	Due(now time.Time, olderThan time.Duration, limit int) []PendingWebpageJob
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
	cutoff := now.Add(-olderThan)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PendingWebpageJob, 0)
	for _, job := range s.jobs {
		if !job.Created.After(cutoff) {
			out = append(out, job)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

type mysqlPendingJobs struct {
	db       *sql.DB
	fallback *memoryPendingJobs
}

func openPendingJobStore() PendingJobStore {
	mem := newMemoryPendingJobs()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		return mem
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logx.Errorf("pending webpage jobs: open: %v", err)
		return mem
	}
	if err = db.Ping(); err != nil {
		logx.Errorf("pending webpage jobs: ping: %v", err)
		_ = db.Close()
		return mem
	}
	if _, err = db.Exec(pendingJobsCreateSQL); err != nil {
		logx.Errorf("pending webpage jobs: create: %v", err)
		_ = db.Close()
		return mem
	}
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
	rows, err := s.db.Query(pendingJobsDueSQL, cutoff, limit)
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
		var created int64
		if err = rows.Scan(&job.UserID, &job.MsgID, &job.PeerType, &job.PeerID, &job.AuthKeyID, &created); err != nil {
			logx.Errorf("pending webpage jobs: scan: %v", err)
			continue
		}
		job.Created = time.Unix(created, 0).UTC()
		out = append(out, job)
	}
	return out
}

// pendingJobs is the process-wide store. Tests replace it.
var pendingJobs PendingJobStore = newMemoryPendingJobs()

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
