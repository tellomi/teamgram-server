// Command expireauthorizations reports logins that are past
// account.setAuthorizationTTL. The default run only prints a count.
//
//	expireauthorizations --dsn 'user:pass@tcp(127.0.0.1:3306)/teamgram'
//
// --apply copies the matching auth_users rows into auth_users_ttl_backup,
// soft-deletes them, and deletes their push-token rows. --rollback RUN
// restores deleted, date_created and date_active from that backup. Push
// tokens are not restored; the device registers again on the next login.
//
// The command does not read an environment DSN and does not print the DSN,
// tokens, or phone numbers. Do not point it at production unless that run
// has been reviewed.
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
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY run_id (run_id)
) ENGINE=InnoDB`

// staleWhere is the same rule as authzttl.Expired.
// date_active <= now - days*86400, and a missing date_active is kept.
const staleWhere = `
au.deleted = 0
AND au.date_active > 0
AND au.date_active <= UNIX_TIMESTAMP() - COALESCE(NULLIF(u.authorization_ttl_days, 0), 180) * 86400`

const countSQL = `
SELECT COUNT(*)
FROM auth_users au
LEFT JOIN users u ON u.id = au.user_id
WHERE ` + staleWhere

const backupSQL = `
INSERT INTO auth_users_ttl_backup
  (run_id, auth_user_id, auth_key_id, user_id, deleted_before, date_created, date_active)
SELECT ?, au.id, au.auth_key_id, au.user_id, au.deleted, au.date_created, au.date_active
FROM auth_users au
LEFT JOIN users u ON u.id = au.user_id
WHERE ` + staleWhere

// expireSQL changes only rows copied into this run, so rollback covers them.
const expireSQL = `
UPDATE auth_users au
JOIN auth_users_ttl_backup b ON b.auth_user_id = au.id
SET au.deleted = 1, au.date_created = 0, au.date_active = 0
WHERE b.run_id = ?`

const deleteTokensSQL = `
DELETE d FROM devices d
JOIN auth_users_ttl_backup b ON b.auth_key_id = d.auth_key_id AND b.user_id = d.user_id
WHERE b.run_id = ?`

const rollbackSQL = `
UPDATE auth_users au
JOIN auth_users_ttl_backup b ON b.auth_user_id = au.id
SET au.deleted = b.deleted_before,
    au.date_created = b.date_created,
    au.date_active = b.date_active
WHERE b.run_id = ?`

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN for the teamgram schema")
	apply := flag.Bool("apply", false, "soft-delete the rows this command would report")
	rollback := flag.Int64("rollback", 0, "restore auth_users from this backup run id")
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
		err = rollbackRun(ctx, db, *rollback)
	case *apply:
		err = applyRun(ctx, db)
	default:
		err = reportRun(ctx, db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func reportRun(ctx context.Context, db *sql.DB) error {
	var n int64
	if err := db.QueryRowContext(ctx, countSQL).Scan(&n); err != nil {
		return err
	}
	fmt.Printf("would_expire=%d\n", n)
	fmt.Println("dry_run=1")
	return nil
}

func applyRun(ctx context.Context, db *sql.DB) error {
	var n int64
	if err := db.QueryRowContext(ctx, countSQL).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		fmt.Println("would_expire=0")
		fmt.Println("applied=0")
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, backupTable); err != nil {
		return err
	}
	runID := time.Now().UnixNano()
	res, err := tx.ExecContext(ctx, backupSQL, runID)
	if err != nil {
		return err
	}
	copied, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, expireSQL, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, deleteTokensSQL, runID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("run_id=%d\n", runID)
	fmt.Printf("expired=%d\n", copied)
	fmt.Println("rollback=expireauthorizations --dsn <dsn> --rollback " + fmt.Sprint(runID))
	return nil
}

func rollbackRun(ctx context.Context, db *sql.DB, runID int64) error {
	res, err := db.ExecContext(ctx, rollbackSQL, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	fmt.Printf("restored=%d\n", n)
	fmt.Println("push_tokens_not_restored=1")
	return nil
}
