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
	"time"

	_ "github.com/go-sql-driver/mysql"
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

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN for the teamgram schema")
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
	switch {
	case *rollback != 0:
		var n int64
		n, err = rollbackRun(ctx, db, *rollback)
		if err == nil {
			fmt.Printf("restored=%d\n", n)
		}
	case *apply:
		var runID, copied int64
		runID, copied, err = applyRun(ctx, db)
		if err == nil {
			fmt.Printf("run_id=%d\n", runID)
			fmt.Printf("expired=%d\n", copied)
			if copied > 0 {
				fmt.Println("rollback=expireauthorizations --dsn <dsn> --rollback " + fmt.Sprint(runID))
			}
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

func applyRun(ctx context.Context, db *sql.DB) (runID, copied int64, err error) {
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
	return runID, copied, nil
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
