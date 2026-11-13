package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/authzttl"

	"github.com/zeromicro/go-zero/core/logx"
)

// ApplyAuthorizationLifetime runs on a real RPC or ping (GetAuthStateData).
// A trusted row is judged on the date_active already stored. If that time is
// past the TTL, this reconnect is a logout: the row is soft-deleted and the
// push token is removed. Refreshing first would make that branch unreachable
// and would keep a single-device account alive forever. An untrusted row is
// the bind-time stamp from before activity was tracked. It is never expired.
// Its first real request stores now and starts the clock.
// now is the caller's clock so tests can pass a fixed time.
func (d *Dao) ApplyAuthorizationLifetime(ctx context.Context, authKeyID int64, cData *CacheAuthData, now int64) {
	if d == nil || cData == nil || cData.Revoked || cData.BindUser == nil || authKeyID == 0 || cData.UserId() == 0 {
		return
	}
	if cData.BindUser.ActivityTrusted {
		ttl, ok := d.authorizationTTLDays(ctx, cData.UserId())
		if !ok {
			return
		}
		last := cData.DateActivated()
		if authzttl.Expired(last, now, ttl) {
			if d.expireAuthorization(ctx, cData.UserId(), authKeyID, last) {
				cData.BindUser = nil
				cData.Revoked = true
				return
			}
			d.reloadAuthorizationActivity(ctx, authKeyID, cData)
			return
		}
	}
	d.noteAuthorizationActivity(ctx, authKeyID, cData, now)
}

type authActivityRow struct {
	AuthKeyID       int64 `db:"auth_key_id"`
	DateActive      int64 `db:"date_active"`
	ActivityTrusted int32 `db:"activity_trusted"`
}

// ExpireStaleAuthorizations drops keys whose tracked activity is past the TTL.
// The caller is skipped. A row that has never recorded a ping or RPC
// (activity_trusted = 0) is kept: its date_active is the bind time.
func (d *Dao) ExpireStaleAuthorizations(ctx context.Context, userID, keepAuthKeyID, now int64) {
	if d == nil || userID == 0 {
		return
	}
	var rows []authActivityRow
	err := d.QueryRowsPartial(ctx, &rows, "select auth_key_id, date_active, activity_trusted from auth_users where user_id = ? and deleted = 0", userID)
	if err != nil {
		logx.WithContext(ctx).Errorf("expire authorizations user_id=%d: %v", userID, err)
		return
	}
	ttl, ok := d.authorizationTTLDays(ctx, userID)
	if !ok {
		return
	}
	for _, row := range rows {
		if row.AuthKeyID == 0 || row.AuthKeyID == keepAuthKeyID || row.ActivityTrusted == 0 {
			continue
		}
		if !authzttl.Expired(row.DateActive, now, ttl) {
			continue
		}
		d.expireAuthorization(ctx, userID, row.AuthKeyID, row.DateActive)
	}
}

// authorizationTTLDays reads the stored window. ok is false when the read
// fails or the user row is missing. Callers must skip expiry in that case.
// Guessing 180 days and deleting would log people out because the database
// was briefly unreadable. A stored 0 still means the column default of 180,
// because that read succeeded.
func (d *Dao) authorizationTTLDays(ctx context.Context, userID int64) (int32, bool) {
	if d == nil || userID == 0 {
		return 0, false
	}
	var row struct {
		Days int32 `db:"authorization_ttl_days"`
	}
	err := d.QueryRowPartial(ctx, &row, "select authorization_ttl_days from users where id = ? limit 1", userID)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) {
			logx.WithContext(ctx).Errorf("authorization ttl user_id=%d: %v", userID, err)
		}
		return 0, false
	}
	return authzttl.Days(row.Days), true
}

// expireAuthorization soft-deletes one trusted row whose date_active is still
// the value that was judged idle (notAfter). A concurrent ping that stored a
// newer date_active matches zero rows, and its push token is left in place.
// The bool is false when nothing was deleted.
func (d *Dao) expireAuthorization(ctx context.Context, userID, authKeyID, notAfter int64) bool {
	if d == nil || userID == 0 || authKeyID == 0 || notAfter <= 0 {
		return false
	}
	var rows int64
	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := lockAuthIdentities(tx, []int64{authKeyID}); err != nil {
			result.Err = err
			return
		}
		res, err := tx.Exec("update auth_users set deleted = 1, date_created = 0, date_active = 0 where auth_key_id = ? and user_id = ? and deleted = 0 and activity_trusted = 1 and date_active > 0 and date_active <= ?", authKeyID, userID, notAfter)
		if err != nil {
			result.Err = err
			return
		}
		rows, err = res.RowsAffected()
		result.Err = err
	})
	if result.Err != nil {
		logx.WithContext(ctx).Errorf("expire authorization user_id=%d auth_key_id=%d: %v", userID, authKeyID, result.Err)
		return false
	}
	if rows == 0 {
		return false
	}
	if err := d.bumpAuthBindGen(ctx, authKeyID); err != nil {
		logx.WithContext(ctx).Errorf("expire authorization cache user_id=%d auth_key_id=%d: %v", userID, authKeyID, err)
	}
	d.deleteDeviceTokens(ctx, userID, []int64{authKeyID})
	return true
}

// noteAuthorizationActivity stores this RPC or ping as date_active.
// An untrusted row is always refreshed, even when the bind-time stamp is
// recent, so the TTL starts at this request. A trusted row is refreshed at
// most once per TouchGap. A zero-row update is re-read so a concurrent touch
// cannot be treated as inactivity.
func (d *Dao) noteAuthorizationActivity(ctx context.Context, authKeyID int64, cData *CacheAuthData, now int64) {
	if d == nil || d.DB == nil || cData == nil || cData.BindUser == nil || authKeyID == 0 {
		return
	}
	last := cData.DateActivated()
	if cData.BindUser.ActivityTrusted && now-last < authzttl.TouchGap {
		return
	}
	cutoff := now - authzttl.TouchGap
	res, err := d.DB.Exec(ctx, "update auth_users set date_active = ?, activity_trusted = 1 where auth_key_id = ? and user_id = ? and deleted = 0 and (activity_trusted = 0 or date_active <= ?)", now, authKeyID, cData.UserId(), cutoff)
	if err != nil {
		logx.WithContext(ctx).Errorf("touch authorization auth_key_id=%d: %v", authKeyID, err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		cData.BindUser.DateActivated = now
		cData.BindUser.ActivityTrusted = true
		if err := d.bumpAuthBindGen(ctx, authKeyID); err != nil {
			logx.WithContext(ctx).Errorf("touch authorization cache auth_key_id=%d: %v", authKeyID, err)
		}
		return
	}
	d.reloadAuthorizationActivity(ctx, authKeyID, cData)
}

func (d *Dao) reloadAuthorizationActivity(ctx context.Context, authKeyID int64, cData *CacheAuthData) {
	var row authActivityRow
	err := d.QueryRowPartial(ctx, &row, "select auth_key_id, date_active, activity_trusted from auth_users where auth_key_id = ? and user_id = ? and deleted = 0 limit 1", authKeyID, cData.UserId())
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			cData.BindUser = nil
			cData.Revoked = true
		}
		return
	}
	cData.BindUser.DateActivated = row.DateActive
	cData.BindUser.ActivityTrusted = row.ActivityTrusted != 0
}

// deleteDeviceTokens removes push registrations for these keys.
// ListByUser already ignores a deleted auth_users row. Deleting the device
// row is what stops a later re-bind of the same key from inheriting the token.
// A missing devices table is logged and does not undo the auth_users update.
func (d *Dao) deleteDeviceTokens(ctx context.Context, userID int64, authKeyIDs []int64) {
	if d == nil || d.DB == nil || userID == 0 || len(authKeyIDs) == 0 {
		return
	}
	query := fmt.Sprintf("delete from devices where user_id = ? and auth_key_id in (%s)", sqlx.InInt64List(authKeyIDs))
	if _, err := d.DB.Exec(ctx, query, userID); err != nil {
		logx.WithContext(ctx).Errorf("delete device tokens user_id=%d: %v", userID, err)
	}
}

// keptAuthorizationHash is the hash stored for this bind.
// A live row keeps the hash the other devices already received from
// account.getAuthorizations. Replacing it makes account.resetAuthorization
// return HASH_INVALID, or revoke a row that is no longer the one on screen.
// A deleted row is a new login and takes proposed, which must be non-zero:
// hash 0 means "this session" and "every other session".
func keptAuthorizationHash(existing int64, deleted bool, proposed int64) int64 {
	if !deleted && existing != 0 {
		return existing
	}
	if proposed != 0 {
		return proposed
	}
	return newAuthorizationHash()
}

// AdoptTempAuthorization moves a login that was stored on a temporary key
// onto the permanent key and drops the temporary row, in one transaction.
// auth.bindTempAuthKey used to leave both rows, so one login showed up twice.
// A retry after a committed move finds no live temp row, deletes any leftover
// temp-key push token, and returns success.
func (d *Dao) AdoptTempAuthorization(ctx context.Context, tempID, permID int64) error {
	if d == nil || tempID == 0 || permID == 0 || tempID == permID {
		return nil
	}
	var (
		uid     int64
		moved   bool
		markNew bool
	)
	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := lockAuthIdentities(tx, []int64{tempID, permID}); err != nil {
			result.Err = err
			return
		}
		var owner struct {
			UserID int64 `db:"user_id"`
		}
		err := tx.QueryRowPartial(&owner, "select user_id from auth_users where auth_key_id = ? and deleted = 0 limit 1", tempID)
		if errors.Is(err, sqlx.ErrNotFound) {
			_, err = tx.Exec("delete from devices where auth_key_id = ?", tempID)
			result.Err = err
			return
		}
		if err != nil {
			result.Err = err
			return
		}
		uid = owner.UserID
		owners, err := activeAuthOwners(tx, permID)
		if err != nil {
			result.Err = err
			return
		}
		if authBindConflicts(owners, uid) {
			result.Err = ErrAuthKeyUserConflict
			return
		}
		hasPerm := false
		for _, id := range owners {
			if id == uid {
				hasPerm = true
				break
			}
		}
		if !hasPerm {
			now := time.Now().Unix()
			do := &dataobject.AuthUsersDO{
				AuthKeyId:   permID,
				UserId:      uid,
				Hash:        newAuthorizationHash(),
				DateCreated: now,
				DateActive:  now,
			}
			var existing struct {
				Hash    int64 `db:"hash"`
				Deleted int32 `db:"deleted"`
			}
			err = tx.QueryRowPartial(&existing, "select hash, deleted from auth_users where auth_key_id = ? and user_id = ? limit 1", permID, uid)
			if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
				result.Err = err
				return
			}
			if err == nil {
				do.Hash = keptAuthorizationHash(existing.Hash, existing.Deleted != 0, do.Hash)
			}
			_, rows, err := d.AuthUsersDAO.InsertOrUpdatesTx(tx, do)
			if err != nil {
				result.Err = err
				return
			}
			markNew = rows == 1
		}
		if _, err = tx.Exec("update auth_users set deleted = 1, date_active = 0 where auth_key_id = ? and user_id = ? and deleted = 0", tempID, uid); err != nil {
			result.Err = err
			return
		}
		if _, err = tx.Exec("delete from devices where auth_key_id = ?", tempID); err != nil {
			result.Err = err
			return
		}
		moved = true
	})
	if result.Err != nil {
		return result.Err
	}
	if err := d.bumpAuthBindGens(ctx, []int64{tempID, permID}); err != nil {
		logx.WithContext(ctx).Errorf("adopt temp authorization cache temp=%d perm=%d: %v", tempID, permID, err)
	}
	if moved && markNew {
		d.markNewAuthorizationUnconfirmed(ctx, uid, permID)
	}
	return nil
}
