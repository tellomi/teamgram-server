// Command push-device-cleanup deletes devices rows whose auth session is
// already logged out or missing. It is safe to run more than once.
//
// Take a backup of the devices table before deleting. The command does not
// delete unless -backup-done is set, and it never prints tokens or secrets.
// See docs/push.md.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/push"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("TEAMGRAM_MYSQL_DSN"), "MySQL DSN. Defaults to TEAMGRAM_MYSQL_DSN. The value is not logged.")
	dryRun := flag.Bool("dry-run", false, "print how many stale rows would be deleted and do not write")
	backupDone := flag.Bool("backup-done", false, "delete stale rows; only after the devices backup in docs/push.md")
	flag.Parse()

	if err := run(context.Background(), strings.TrimSpace(*dsn), *dryRun, *backupDone); err != nil {
		fmt.Fprintln(os.Stderr, "push-device-cleanup:", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, dsn string, dryRun, backupDone bool) error {
	if dsn == "" {
		return fmt.Errorf("missing -dsn or TEAMGRAM_MYSQL_DSN")
	}
	apply := backupDone && !dryRun
	if dryRun {
		fmt.Fprintln(os.Stderr, "dry-run: no rows will be deleted. To delete after the devices backup, pass -backup-done. See docs/push.md.")
	} else if !backupDone {
		fmt.Fprintln(os.Stderr, backupInstructions())
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	matched, deleted, err := push.CleanupStaleDevices(ctx, db, apply)
	if err != nil {
		return err
	}
	if !apply {
		fmt.Printf("push-device-cleanup: dry-run stale=%d (no rows deleted)\n", matched)
		return nil
	}
	fmt.Printf("push-device-cleanup: deleted=%d\n", deleted)
	return nil
}

func backupInstructions() string {
	return strings.TrimSpace(`
Refusing to delete devices until you pass -backup-done.
Back up the table first. The steps are in docs/push.md. Do not commit the dump; it holds device tokens and secrets.

  mysqldump --single-transaction --no-tablespaces teamgram devices \
    > "devices-before-push-cleanup-$(date -u +%Y%m%dT%H%M%SZ).sql"

Then, with TEAMGRAM_MYSQL_DSN set or -dsn:

  go run ./cmd/push-device-cleanup -backup-done

A second run deletes nothing. -dry-run only prints the count.
`)
}
