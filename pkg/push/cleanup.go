package push

import (
	"context"
	"database/sql"
	"fmt"
)

// staleDeviceWhere matches device rows whose auth_users binding is missing
// or logged out (deleted = 1). It is the same condition ListByUser uses to
// refuse a push. Running the delete twice removes nothing the second time.
const staleDeviceWhere = `NOT EXISTS (
  SELECT 1 FROM auth_users au
  WHERE au.auth_key_id = d.auth_key_id AND au.user_id = d.user_id AND au.deleted = 0
)`

// CleanupStaleDevices counts device rows whose login is gone.
// apply false does not write. apply true deletes those rows.
// The caller takes a backup of `devices` before apply; this function does
// not print tokens or secrets.
func CleanupStaleDevices(ctx context.Context, db *sql.DB, apply bool) (matched, deleted int64, err error) {
	if db == nil {
		return 0, 0, fmt.Errorf("push cleanup: nil db")
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices d WHERE `+staleDeviceWhere).Scan(&matched); err != nil {
		return 0, 0, err
	}
	if !apply || matched == 0 {
		return matched, 0, nil
	}
	res, err := db.ExecContext(ctx, `DELETE d FROM devices d WHERE `+staleDeviceWhere)
	if err != nil {
		return matched, 0, err
	}
	deleted, err = res.RowsAffected()
	if err != nil {
		return matched, 0, err
	}
	return matched, deleted, nil
}
