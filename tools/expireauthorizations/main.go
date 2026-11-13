// Command expireauthorizations reports logins whose tracked activity is past
// account.setAuthorizationTTL. The default run only prints a count.
//
//	expireauthorizations --dsn 'user:pass@tcp(127.0.0.1:3306)/teamgram'
//
// A row counts only when activity_trusted = 1. That flag is set by a ping or
// RPC. date_active left from bind, before activity was tracked, is not
// inactivity and is not deleted.
//
// --apply copies the matching auth_users rows and their device tokens into
// backup tables, soft-deletes the authorizations, and deletes those tokens.
// The backup tables are created before the transaction. MySQL commits
// implicitly on CREATE TABLE, so creating them inside the transaction would
// commit the deletes early. --rollback RUN restores the authorization columns
// and the device rows in one transaction.
//
// The command does not read an environment DSN and does not print the DSN,
// tokens, or phone numbers. Do not point it at production unless that run
// has been reviewed. auth_users.activity_trusted must already exist
// (migrate-20261112-authorization-activity.sql).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

const backupTable = `
CREATE TABLE IF NOT EXISTS auth_users_ttl_backup (
  id BIGINT NOT NULL AUTO_INCREMENT,
  run_id BIGINT NOT NULL,
  auth_user_id BIGINT NOT NULL,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  deleted_before TINYINT NOT NULL,
  date_created BIGINT NOT NULL,
  date_active BIGINT NOT NULL,
  activity_trusted TINYINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY run_id (run_id)
) ENGINE=InnoDB`

const deviceBackupTable = `
CREATE TABLE IF NOT EXISTS auth_users_ttl_device_backup (
  id BIGINT NOT NULL AUTO_INCREMENT,
  run_id BIGINT NOT NULL,
  device_id BIGINT NOT NULL,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  token_type INT NOT NULL,
  token VARCHAR(512) NOT NULL,
  no_muted TINYINT NOT NULL,
  locked_period INT NOT NULL,
  app_sandbox TINYINT NOT NULL,
  secret VARCHAR(1024) NOT NULL,
  other_uids VARCHAR(1024) NOT NULL,
  lang_code VARCHAR(32) NOT NULL,
  state TINYINT NOT NULL,
  PRIMARY KEY (id),
  KEY run_id (run_id)
) ENGINE=InnoDB`

// staleWhere matches authzttl.Expired for rows whose date_active was stored
// by a ping or RPC. Untrusted bind times are kept.
const staleWhere = `
au.deleted = 0
AND au.activity_trusted = 1
AND au.date_active > 0
AND au.date_active <= UNIX_TIMESTAMP() - COALESCE(NULLIF(u.authorization_ttl_days, 0), 180) * 86400`

const countSQL = `
SELECT COUNT(*)
FROM auth_users au
LEFT JOIN users u ON u.id = au.user_id
WHERE ` + staleWhere

const backupSQL = `
INSERT INTO auth_users_ttl_backup
  (run_id, auth_user_id, auth_key_id, user_id, deleted_before, date_created, date_active, activity_trusted)
SELECT ?, au.id, au.auth_key_id, au.user_id, au.deleted, au.date_created, au.date_active, au.activity_trusted
FROM auth_users au
LEFT JOIN users u ON u.id = au.user_id
WHERE ` + staleWhere

const deviceBackupSQL = `
INSERT INTO auth_users_ttl_device_backup
  (run_id, device_id, auth_key_id, user_id, token_type, token, no_muted, locked_period, app_sandbox, secret, other_uids, lang_code, state)
SELECT ?, d.id, d.auth_key_id, d.user_id, d.token_type, d.token, d.no_muted, d.locked_period, d.app_sandbox, d.secret, d.other_uids, d.lang_code, d.state
FROM devices d
JOIN auth_users_ttl_backup b ON b.auth_key_id = d.auth_key_id AND b.user_id = d.user_id
WHERE b.run_id = ?`

// expireSQL changes only rows copied into this run, so rollback covers them.
const expireSQL = `
UPDATE auth_users au
JOIN auth_users_ttl_backup b ON b.auth_user_id = au.id
SET au.deleted = 1, au.date_created = 0, au.date_active = 0
WHERE b.run_id = ?`

const deleteTokensSQL = `
DELETE d FROM devices d
JOIN auth_users_ttl_device_backup bk ON bk.device_id = d.id
WHERE bk.run_id = ?`

const rollbackSQL = `
UPDATE auth_users au
JOIN auth_users_ttl_backup b ON b.auth_user_id = au.id
SET au.deleted = b.deleted_before,
    au.date_created = b.date_created,
    au.date_active = b.date_active,
    au.activity_trusted = b.activity_trusted
WHERE b.run_id = ?`

const restoreTokensSQL = `
INSERT INTO devices
  (id, auth_key_id, user_id, token_type, token, no_muted, locked_period, app_sandbox, secret, other_uids, lang_code, state)
SELECT device_id, auth_key_id, user_id, token_type, token, no_muted, locked_period, app_sandbox, secret, other_uids, lang_code, state
FROM auth_users_ttl_device_backup
WHERE run_id = ?`

// genCache is the auth_bind_gen / auth_data.2 Redis the session service reads.
// INCR and DEL use the same key text as authsession's bumpAuthBindGen.
type genCache interface {
	Get(ctx context.Context, key string) (string, error)
	Incr(ctx context.Context, key string) (int64, error)
	Del(ctx context.Context, keys ...string) error
}

type redisGenCache struct {
	client *redis.Client
}

func (c redisGenCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (c redisGenCache) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

func (c redisGenCache) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN for the teamgram schema")
	redisAddr := flag.String("redis", "", "Redis host:port that holds auth_bind_gen; required with --apply")
	apply := flag.Bool("apply", false, "soft-delete the rows this command would report")
	rollback := flag.Int64("rollback", 0, "restore auth_users and device tokens from this backup run id")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "dsn is required")
		os.Exit(2)
	}
	if *apply && *rollback != 0 {
		fmt.Fprintln(os.Stderr, "choose either --apply or --rollback")
		os.Exit(2)
	}
	if *apply && *redisAddr == "" {
		fmt.Fprintln(os.Stderr, "redis is required with --apply so revoked keys leave the auth cache")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err.Error())
		os.Exit(1)
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err.Error())
		os.Exit(1)
	}
	var cache genCache
	if *redisAddr != "" {
		client := redis.NewClient(&redis.Options{Addr: *redisAddr})
		defer client.Close()
		if err = client.Ping(ctx).Err(); err != nil {
			fmt.Fprintln(os.Stderr, "redis:", err.Error())
			os.Exit(1)
		}
		cache = redisGenCache{client: client}
	}
	switch {
	case *rollback != 0:
		var n int64
		n, err = rollbackRun(ctx, db, *rollback)
		if err == nil {
			fmt.Printf("restored=%d\n", n)
		}
	case *apply:
		var runID, copied int64
		runID, copied, err = applyRun(ctx, db, cache)
		fmt.Printf("run_id=%d\n", runID)
		fmt.Printf("expired=%d\n", copied)
		if err == nil && copied > 0 {
			fmt.Println("rollback=expireauthorizations --dsn <dsn> --rollback " + fmt.Sprint(runID))
		}
	default:
		var n int64
		n, err = countStale(ctx, db)
		if err == nil {
			fmt.Printf("would_expire=%d\n", n)
			fmt.Println("dry_run=1")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func countStale(ctx context.Context, db *sql.DB) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, countSQL).Scan(&n)
	return n, err
}

// ensureBackupTables creates the backup tables before any transaction.
// CREATE TABLE commits the current transaction on MySQL, so it must not run
// after Begin.
func ensureBackupTables(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, backupTable); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, deviceBackupTable)
	return err
}

func applyRun(ctx context.Context, db *sql.DB, cache genCache) (runID, copied int64, err error) {
	n, err := countStale(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	if n == 0 {
		return 0, 0, nil
	}
	if err = ensureBackupTables(ctx, db); err != nil {
		return 0, 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	runID = time.Now().UnixNano()
	res, err := tx.ExecContext(ctx, backupSQL, runID)
	if err != nil {
		return 0, 0, err
	}
	copied, err = res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, deviceBackupSQL, runID, runID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, expireSQL, runID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, deleteTokensSQL, runID); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	if cache == nil {
		return runID, copied, nil
	}
	ids, err := backupKeyIDs(ctx, db, runID)
	if err != nil {
		return runID, copied, err
	}
	if err = bumpAuthGens(ctx, cache, ids); err != nil {
		return runID, copied, err
	}
	return runID, copied, nil
}

func backupKeyIDs(ctx context.Context, db *sql.DB, runID int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx, "SELECT auth_key_id FROM auth_users_ttl_backup WHERE run_id = ?", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// bumpAuthGens matches authsession bumpAuthBindGen. The generation moves
// forward and the previous auth_data.2 slot is deleted, so a gateway that
// still has the logged-in snapshot cannot keep serving it.
func bumpAuthGens(ctx context.Context, cache genCache, ids []int64) error {
	var first error
	for _, id := range ids {
		if id == 0 || cache == nil {
			continue
		}
		genKey := fmt.Sprintf("auth_bind_gen#%d", id)
		raw, err := cache.Get(ctx, genKey)
		prev := int64(0)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if raw != "" {
			prev, err = strconv.ParseInt(raw, 10, 64)
			if err != nil {
				if first == nil {
					first = err
				}
				continue
			}
		}
		slot := fmt.Sprintf("auth_data.2#%d#%d", id, prev)
		if _, err = cache.Incr(ctx, genKey); err != nil {
			if first == nil {
				first = err
			}
			_ = cache.Del(ctx, slot)
			continue
		}
		if err = cache.Del(ctx, slot); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func rollbackRun(ctx context.Context, db *sql.DB, runID int64) (int64, error) {
	if err := ensureBackupTables(ctx, db); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, rollbackSQL, runID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, restoreTokensSQL, runID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
