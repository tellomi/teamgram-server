package dao

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/teamgram/marmota/pkg/stores/sqlx"

	"github.com/zeromicro/go-zero/core/logx"
)

const authBindGenPrefix = "auth_bind_gen"

// ErrAuthKeyUserConflict means this permanent auth key already has an active
// auth_users owner other than the requested user. The bind transaction rolls
// back, so the row is not inserted or updated and the cache is not touched.
var ErrAuthKeyUserConflict = errors.New("auth key already bound to a different user")

// ErrAuthKeyNotRegistered means neither auth_keys nor auth_key_infos has a
// row for this id. Those unique rows are the lock that serializes two
// first-time binds. Without one, two different users can both insert.
var ErrAuthKeyNotRegistered = errors.New("auth key is not registered")

var errAuthGenUnavailable = errors.New("auth bind generation store is unavailable")

// authLoadGate, when set, runs after GetCacheAuthData has loaded its DB
// snapshot and before that snapshot is written to cache. Tests block here
// so a writer can commit and move the generation first. Production leaves
// it unset. The value is a func().
var authLoadGate atomic.Value

func waitAuthLoadGate() {
	fn, _ := authLoadGate.Load().(func())
	if fn != nil {
		fn()
	}
}

func genAuthBindGenKey(id int64) string {
	return fmt.Sprintf("%s#%d", authBindGenPrefix, id)
}

// genAuthDataCacheKey is the cache slot for one generation of auth_data.
// Readers sample the generation before they read MySQL and write only this
// slot. A writer increments the generation after its SQL commits, so a
// reader that still holds the previous snapshot cannot put that snapshot
// back into the slot new readers use.
func genAuthDataCacheKey(id, gen int64) string {
	return fmt.Sprintf("%s#%d#%d", authDataPrefix, id, gen)
}

// authBindConflicts is true when any active owner is a different user.
// Revoked rows are not active and are not passed in. The same user, or no
// active row, is not a conflict: InsertOrUpdates revives that user's row
// or inserts one.
func authBindConflicts(active []int64, userID int64) bool {
	for _, id := range active {
		if id != userID {
			return true
		}
	}
	return false
}

type authOwnerRow struct {
	UserID int64 `db:"user_id"`
}

func activeAuthOwners(tx *sqlx.Tx, authKeyID int64) ([]int64, error) {
	var rows []authOwnerRow
	err := tx.QueryRowsPartial(&rows, "select user_id from auth_users where auth_key_id = ? and deleted = 0", authKeyID)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.UserID)
	}
	return out, nil
}

// lockAuthIdentity locks the unique row that identifies this auth key.
// auth_users is unique on (auth_key_id, user_id), so locking only that pair
// lets two different users insert together. auth_keys (and, for a legacy
// key that was never copied into auth_keys, auth_key_infos) is one row per
// key even when auth_users has no row yet.
func lockAuthIdentity(tx *sqlx.Tx, authKeyID int64) error {
	var row struct {
		AuthKeyID int64 `db:"auth_key_id"`
	}
	err := tx.QueryRowPartial(&row, "select auth_key_id from auth_keys where auth_key_id = ? for update", authKeyID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sqlx.ErrNotFound) {
		return err
	}
	err = tx.QueryRowPartial(&row, "select auth_key_id from auth_key_infos where auth_key_id = ? for update", authKeyID)
	if err == nil {
		return nil
	}
	if errors.Is(err, sqlx.ErrNotFound) {
		return ErrAuthKeyNotRegistered
	}
	return err
}

func sortedKeyIDs(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// lockAuthIdentities locks each key in id order. A missing key row does not
// abort logout: the auth_users delete still has to run. Any other error
// aborts so the transaction can roll back.
func lockAuthIdentities(tx *sqlx.Tx, ids []int64) error {
	for _, id := range sortedKeyIDs(ids) {
		err := lockAuthIdentity(tx, id)
		if err != nil && !errors.Is(err, ErrAuthKeyNotRegistered) {
			return err
		}
	}
	return nil
}

func (d *Dao) authBindGen(ctx context.Context, authKeyID int64) (int64, error) {
	if d == nil || d.kv == nil {
		return 0, errAuthGenUnavailable
	}
	raw, err := d.kv.GetCtx(ctx, genAuthBindGenKey(authKeyID))
	if err != nil {
		return 0, err
	}
	if raw == "" {
		return 0, nil
	}
	gen, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("auth bind gen %q: %w", raw, err)
	}
	return gen, nil
}

// bumpAuthBindGen moves the live cache slot forward after a successful SQL
// write. The previous slot is deleted so it does not linger; a reader that
// already sampled it may write it again, and new readers will not look there.
// If the increment fails, the current slot is deleted so the next read
// reloads from MySQL instead of keeping the pre-write owner.
func (d *Dao) bumpAuthBindGen(ctx context.Context, authKeyID int64) error {
	if authKeyID == 0 || d == nil || d.kv == nil {
		return errAuthGenUnavailable
	}
	prev, prevErr := d.authBindGen(ctx, authKeyID)
	if _, err := d.kv.IncrCtx(ctx, genAuthBindGenKey(authKeyID)); err != nil {
		if prevErr == nil {
			_ = d.CachedConn.DelCache(ctx, genAuthDataCacheKey(authKeyID, prev))
		}
		logx.WithContext(ctx).Errorf("auth bind gen incr auth_key_id=%d: %v", authKeyID, err)
		return err
	}
	if prevErr == nil {
		if err := d.CachedConn.DelCache(ctx, genAuthDataCacheKey(authKeyID, prev)); err != nil {
			logx.WithContext(ctx).Errorf("auth bind gen drop old cache auth_key_id=%d gen=%d: %v", authKeyID, prev, err)
		}
	}
	return nil
}

func (d *Dao) bumpAuthBindGens(ctx context.Context, ids []int64) error {
	var first error
	for _, id := range ids {
		if err := d.bumpAuthBindGen(ctx, id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// execAuthData runs a successful auth_data write and then moves the cache
// generation. A failed write leaves the generation and the cache alone.
func (d *Dao) execAuthData(ctx context.Context, authKeyID int64, exec func(ctx context.Context, conn *sqlx.DB) (int64, int64, error)) (int64, int64, error) {
	last, rows, err := exec(ctx, d.DB)
	if err != nil {
		return last, rows, err
	}
	if ferr := d.bumpAuthBindGen(ctx, authKeyID); ferr != nil {
		return last, rows, ferr
	}
	return last, rows, nil
}
