package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/pkg/authzttl"

	"github.com/zeromicro/go-zero/core/logx"
)

// ApplyAuthorizationLifetime enforces account.setAuthorizationTTL on one key.
// An inactive binding is soft-deleted and the cache copy is marked revoked so
// this request already fails. A live binding refreshes date_active at most
// once per TouchGap. now is the caller's clock so tests can pass a fixed time.
func (d *Dao) ApplyAuthorizationLifetime(ctx context.Context, authKeyID int64, cData *CacheAuthData, now int64) {
	if d == nil || cData == nil || cData.Revoked || cData.BindUser == nil || authKeyID == 0 || cData.UserId() == 0 {
		return
	}
	ttl := d.authorizationTTLDays(ctx, cData.UserId())
	if authzttl.Expired(cData.DateActivated(), now, ttl) {
		d.expireAuthorization(ctx, cData.UserId(), authKeyID)
		cData.BindUser = nil
		cData.Revoked = true
		return
	}
	d.touchAuthorization(ctx, authKeyID, cData, now)
}

// ExpireStaleAuthorizations drops this user's inactive keys before the device
// list is built. The caller's own key is left for ApplyAuthorizationLifetime,
// which already ran on this request and would have revoked it if it were idle.
func (d *Dao) ExpireStaleAuthorizations(ctx context.Context, userID, keepAuthKeyID, now int64) {
	if d == nil || userID == 0 {
		return
	}
	rows, err := d.AuthUsersDAO.SelectListByUserId(ctx, userID)
	if err != nil {
		logx.WithContext(ctx).Errorf("expire authorizations user_id=%d: %v", userID, err)
		return
	}
	ttl := d.authorizationTTLDays(ctx, userID)
	var stale []int64
	for i := range rows {
		if rows[i].AuthKeyId == 0 || rows[i].AuthKeyId == keepAuthKeyID {
			continue
		}
		if authzttl.Expired(rows[i].DateActive, now, ttl) {
			stale = append(stale, rows[i].AuthKeyId)
		}
	}
	for _, id := range stale {
		d.expireAuthorization(ctx, userID, id)
	}
}

func (d *Dao) authorizationTTLDays(ctx context.Context, userID int64) int32 {
	if d == nil || userID == 0 {
		return authzttl.DefaultDays
	}
	var row struct {
		Days int32 `db:"authorization_ttl_days"`
	}
	err := d.QueryRowPartial(ctx, &row, "select authorization_ttl_days from users where id = ? limit 1", userID)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) {
			logx.WithContext(ctx).Errorf("authorization ttl user_id=%d: %v", userID, err)
		}
		return authzttl.DefaultDays
	}
	return authzttl.Days(row.Days)
}

func (d *Dao) expireAuthorization(ctx context.Context, userID, authKeyID int64) {
	if d == nil || userID == 0 || authKeyID == 0 {
		return
	}
	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := lockAuthIdentities(tx, []int64{authKeyID}); err != nil {
			result.Err = err
			return
		}
		_, err := tx.Exec("update auth_users set deleted = 1, date_created = 0, date_active = 0 where auth_key_id = ? and user_id = ? and deleted = 0", authKeyID, userID)
		result.Err = err
	})
	if result.Err != nil {
		logx.WithContext(ctx).Errorf("expire authorization user_id=%d auth_key_id=%d: %v", userID, authKeyID, result.Err)
		return
	}
	if err := d.bumpAuthBindGen(ctx, authKeyID); err != nil {
		logx.WithContext(ctx).Errorf("expire authorization cache user_id=%d auth_key_id=%d: %v", userID, authKeyID, err)
	}
	d.deleteDeviceTokens(ctx, userID, []int64{authKeyID})
}

func (d *Dao) touchAuthorization(ctx context.Context, authKeyID int64, cData *CacheAuthData, now int64) {
	if d == nil || d.DB == nil || cData == nil || cData.BindUser == nil || authKeyID == 0 {
		return
	}
	last := cData.DateActivated()
	if now-last < authzttl.TouchGap {
		return
	}
	cutoff := now - authzttl.TouchGap
	res, err := d.DB.Exec(ctx, "update auth_users set date_active = ? where auth_key_id = ? and user_id = ? and deleted = 0 and date_active <= ?", now, authKeyID, cData.UserId(), cutoff)
	if err != nil {
		logx.WithContext(ctx).Errorf("touch authorization auth_key_id=%d: %v", authKeyID, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return
	}
	cData.BindUser.DateActivated = now
	if err := d.bumpAuthBindGen(ctx, authKeyID); err != nil {
		logx.WithContext(ctx).Errorf("touch authorization cache auth_key_id=%d: %v", authKeyID, err)
	}
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
// onto the permanent key and drops the temporary row. auth.bindTempAuthKey
// used to leave both rows, so one login showed up twice and resetting the
// row on screen did not always name the key the gateway was still using.
func (d *Dao) AdoptTempAuthorization(ctx context.Context, tempID, permID int64) error {
	if d == nil || tempID == 0 || permID == 0 || tempID == permID {
		return nil
	}
	uid := d.GetAuthKeyUserId(ctx, tempID)
	if uid == 0 {
		return nil
	}
	exist := d.GetAuthKeyUserId(ctx, permID)
	if exist != 0 && exist != uid {
		return ErrAuthKeyUserConflict
	}
	if exist == 0 {
		if _, err := d.BindAuthKeyUser(ctx, permID, uid); err != nil {
			return err
		}
	}
	if !d.UnbindAuthUser(ctx, tempID, uid) {
		return fmt.Errorf("unbind temp auth key %d", tempID)
	}
	d.deleteDeviceTokens(ctx, uid, []int64{tempID})
	return nil
}
