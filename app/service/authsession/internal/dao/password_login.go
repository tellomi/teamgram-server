package dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

// PasswordLoginUser is the user id waiting to pass a cloud password on this
// auth key. The row lives in user_password_login, written by auth.signIn
// before the key is bound. A missing table or a missing row is not an error:
// the key is a normal login.
func (d *Dao) PasswordLoginUser(ctx context.Context, authKeyID int64) (int64, bool) {
	if d == nil || d.DB == nil || authKeyID == 0 {
		return 0, false
	}
	var row struct {
		UserId int64 `db:"user_id"`
	}
	err := d.DB.QueryRow(ctx, &row, "select user_id from user_password_login where auth_key_id = ? limit 1", authKeyID)
	if err != nil {
		if err != sqlx.ErrNotFound {
			// The migration may not have run yet. Do not fail every login.
			return 0, false
		}
		return 0, false
	}
	if row.UserId == 0 {
		return 0, false
	}
	return row.UserId, true
}
