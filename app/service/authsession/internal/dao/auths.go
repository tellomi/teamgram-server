// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

const (
	// authDataPrefix is the auth_data cache. The live key also carries the
	// bind generation (see genAuthDataCacheKey). A pre-generation entry
	// auth_data.2#id is not read; the next lookup misses and loads MySQL.
	authDataPrefix = "auth_data.2"
)

//
//func genAuthUserCacheKey(id int64) string {
//	return fmt.Sprintf("%s_%d", authUsersTablePrefix, id)
//}

type BindUser struct {
	UserId               int64 `json:"user_id"`
	Hash                 int64 `json:"hash"`
	DateCreated          int64 `json:"date_created"`
	DateActivated        int64 `json:"date_activated"`
	AndroidPushSessionId int64 `json:"android_push_sessionId"`
	// ActivityTrusted is set once a ping or RPC has stored date_active.
	// A bind-time date_active is not trusted and is not inactivity.
	ActivityTrusted bool `json:"activity_trusted,omitempty"`
}

type CacheAuthData struct {
	Client   *authsession.ClientSession `json:"client"`
	BindUser *BindUser                  `json:"bind_user,omitempty"`
	// Revoked is set when auth_users was soft-deleted (reset or logout).
	// The key must not come back as a fresh unauthorized session.
	Revoked bool `json:"revoked,omitempty"`
}

func (c *CacheAuthData) ToAuthState() (state int) {
	state = mtproto.AuthStateUnknown

	if c == nil {
		state = mtproto.AuthStateNew
		return
	}

	if c.BindUser != nil {
		// A bound user stays authorized even when the client-session row
		// has not been written yet (initConnection on a temp key).
		state = mtproto.AuthStateNormal
	} else if c.Revoked {
		// auth.resetAuthorizations / account.resetAuthorization / logOut
		// soft-delete the binding. The next RPC must fail as logged out.
		state = mtproto.AuthStateLogout
	} else if c.Client == nil {
		state = mtproto.AuthStateWaitInit
	} else {
		state = mtproto.AuthStateUnauthorized
	}

	return
}

func (c *CacheAuthData) GetClient() *authsession.ClientSession {
	if c == nil {
		return nil
	}
	return c.Client
}

func (c *CacheAuthData) AuthKeyId() int64 {
	return c.GetClient().GetAuthKeyId()
}

func (c *CacheAuthData) Layer() int32 {
	return c.GetClient().GetLayer()
}

func (c *CacheAuthData) ApiId() int32 {
	return c.GetClient().GetApiId()
}

func (c *CacheAuthData) DeviceModel() string {
	return c.GetClient().GetDeviceModel()
}

func (c *CacheAuthData) SystemVersion() string {
	return c.GetClient().GetSystemVersion()
}

func (c *CacheAuthData) AppVersion() string {
	return c.GetClient().GetAppVersion()
}

func (c *CacheAuthData) SystemLangCode() string {
	return c.GetClient().GetSystemLangCode()
}

func (c *CacheAuthData) LangPack() string {
	return c.GetClient().GetLangPack()
}

func (c *CacheAuthData) LangCode() string {
	return c.GetClient().GetLangCode()
}

func (c *CacheAuthData) ClientIp() string {
	return c.GetClient().GetIp()
}

func (c *CacheAuthData) Proxy() string {
	return c.GetClient().GetProxy()
}

func (c *CacheAuthData) Params() string {
	return c.GetClient().GetParams()
}

func (c *CacheAuthData) UserId() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.UserId
}

func (c *CacheAuthData) DateCreated() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.DateCreated
}

func (c *CacheAuthData) DateActivated() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.DateActivated
}

func (c *CacheAuthData) Hash() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.Hash
}

func (c *CacheAuthData) AndroidPushSessionId() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.AndroidPushSessionId
}

func (d *Dao) GetApiLayer(ctx context.Context, authKeyId int64) int32 {
	cData, err := d.GetCacheAuthData(ctx, authKeyId)
	if err != nil {
		logx.WithContext(ctx).Errorf("not find layer - keyId = %d", authKeyId)
		return 0
	}
	return cData.Layer()
}

func (d *Dao) GetLangCode(ctx context.Context, authKeyId int64) string {
	cData, err := d.GetCacheAuthData(ctx, authKeyId)
	if err != nil {
		logx.WithContext(ctx).Errorf("not find lang_code - keyId = %d", authKeyId)
		return "en"
	}
	return cData.LangCode()
}

func (d *Dao) GetLangPack(ctx context.Context, authKeyId int64) string {
	cData, err := d.GetCacheAuthData(ctx, authKeyId)
	if err != nil {
		logx.WithContext(ctx).Errorf("not find lang_pack - keyId = %d", authKeyId)
		return ""
	}

	c := cData.LangPack()
	if c == "" {
		if strings.HasSuffix(cData.AppVersion(), " A") {
			c = "weba"
		} else if strings.HasSuffix(cData.AppVersion(), " Z") {
			c = "weba"
		}
	}
	return c
}

func (d *Dao) GetClient(ctx context.Context, authKeyId int64) string {
	cData, err := d.GetCacheAuthData(ctx, authKeyId)
	if err != nil {
		logx.WithContext(ctx).Errorf("not find client - keyId = %d", authKeyId)
		return ""
	}
	c := cData.LangPack()
	if c == "android" {
		if strings.Index(cData.AppVersion(), "TDLib") >= 0 {
			c = "react"
		}
	} else if c == "" {
		if strings.HasSuffix(cData.AppVersion(), " A") {
			c = "weba"
		} else if strings.HasSuffix(cData.AppVersion(), " Z") {
			c = "weba"
		}
	}
	return c
}

func (d *Dao) GetAuthKeyUserId(ctx context.Context, authKeyId int64) int64 {
	cData, _ := d.GetCacheAuthData(ctx, authKeyId)
	// do, _ := d.AuthUsersDAO.Select(ctx, authKeyId)
	if cData == nil {
		logx.WithContext(ctx).Errorf("not find user - keyId = %d", authKeyId)
		return 0
	}

	return cData.UserId()
}

// newAuthorizationHash is the device hash account.getAuthorizations returns.
// Zero is reserved for the current session and for "every other session".
func newAuthorizationHash() int64 {
	for {
		hash := rand.Int63()
		if hash != 0 {
			return hash
		}
	}
}

func (d *Dao) BindAuthKeyUser(ctx context.Context, authKeyId int64, userId int64) (int64, error) {
	now := time.Now().Unix()
	authUsersDO := &dataobject.AuthUsersDO{
		AuthKeyId:   authKeyId,
		UserId:      userId,
		Hash:        newAuthorizationHash(),
		DateCreated: now,
		DateActive:  now,
	}

	var (
		rowsAffected int64
		storedHash   int64
	)
	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := lockAuthIdentity(tx, authKeyId); err != nil {
			result.Err = err
			return
		}
		owners, err := activeAuthOwners(tx, authKeyId)
		if err != nil {
			result.Err = err
			return
		}
		// Another live owner blocks the write. A revoked row does not: the
		// same user's row is revived below, and another user's revoked row
		// stays revoked while this user gets a new row.
		if authBindConflicts(owners, userId) {
			result.Err = ErrAuthKeyUserConflict
			return
		}
		var existing struct {
			Hash    int64 `db:"hash"`
			Deleted int32 `db:"deleted"`
		}
		err = tx.QueryRowPartial(&existing, "select hash, deleted from auth_users where auth_key_id = ? and user_id = ? limit 1", authKeyId, userId)
		if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
			result.Err = err
			return
		}
		if err == nil {
			authUsersDO.Hash = keptAuthorizationHash(existing.Hash, existing.Deleted != 0, authUsersDO.Hash)
		}
		_, rows, err := d.AuthUsersDAO.InsertOrUpdatesTx(tx, authUsersDO)
		if err != nil {
			result.Err = err
			return
		}
		rowsAffected = rows
		var stored struct {
			Hash int64 `db:"hash"`
		}
		if err = tx.QueryRowPartial(&stored, "select hash from auth_users where auth_key_id = ? and user_id = ? limit 1", authKeyId, userId); err != nil {
			result.Err = err
			return
		}
		storedHash = stored.Hash
	})
	if result.Err != nil {
		return 0, result.Err
	}
	authUsersDO.Hash = storedHash
	// The SQL is committed. Move the cache generation before anything else
	// so an in-flight reader of the previous snapshot cannot republish it.
	// A fence failure still leaves the row bound; the next read deletes the
	// old slot and reloads. Returning that failure would look like the bind
	// did not happen.
	if err := d.bumpAuthBindGen(ctx, authKeyId); err != nil {
		logx.WithContext(ctx).Errorf("bindAuthKeyUser cache fence auth_key_id=%d: %v", authKeyId, err)
	}
	// rows == 1 is an insert. ON DUPLICATE KEY UPDATE of an existing
	// key reports 2 and keeps the flags already stored on that row.
	if rowsAffected == 1 {
		d.markNewAuthorizationUnconfirmed(ctx, userId, authKeyId)
	}
	return authUsersDO.Hash, nil
}

func (d *Dao) UnbindAuthUser(ctx context.Context, authKeyId int64, userId int64) bool {
	if authKeyId == 0 {
		var ids []int64
		d.AuthUsersDAO.SelectAuthKeyIdsWithCB(
			ctx,
			userId,
			func(sz, i int, v *dataobject.AuthUsersDO) {
				ids = append(ids, v.AuthKeyId)
			})
		if len(ids) == 0 {
			return true
		}
		result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
			if err := lockAuthIdentities(tx, ids); err != nil {
				result.Err = err
				return
			}
			_, err := d.AuthUsersDAO.DeleteUserTx(tx, userId)
			result.Err = err
		})
		if result.Err != nil {
			return false
		}
		if d.bumpAuthBindGens(ctx, ids) != nil {
			return false
		}
		d.deleteDeviceTokens(ctx, userId, ids)
		return true
	}

	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := lockAuthIdentity(tx, authKeyId); err != nil && !errors.Is(err, ErrAuthKeyNotRegistered) {
			result.Err = err
			return
		}
		_, err := d.AuthUsersDAO.DeleteTx(tx, authKeyId, userId)
		result.Err = err
	})
	if result.Err != nil {
		return false
	}
	if d.bumpAuthBindGen(ctx, authKeyId) != nil {
		return false
	}
	d.deleteDeviceTokens(ctx, userId, []int64{authKeyId})
	return true
}

func (d *Dao) SetClientSessionInfo(ctx context.Context, session *authsession.ClientSession) error {
	_, _, err := d.execAuthData(
		ctx,
		session.GetAuthKeyId(),
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			do := &dataobject.AuthsDO{
				AuthKeyId:      session.GetAuthKeyId(),
				Layer:          session.GetLayer(),
				ApiId:          session.GetApiId(),
				DeviceModel:    session.GetDeviceModel(),
				SystemVersion:  session.GetSystemVersion(),
				AppVersion:     session.GetAppVersion(),
				SystemLangCode: session.GetSystemLangCode(),
				LangPack:       session.GetLangPack(),
				LangCode:       session.GetLangCode(),
				ClientIp:       session.GetIp(),
				Proxy:          session.GetProxy(),
				Params:         session.GetParams(),
				DateActive:     time.Now().Unix(),
			}
			if do.Params == "" {
				do.Params = "null"
			}
			return d.AuthsDAO.InsertOrUpdate(ctx, do)
		})

	return err
}

func (d *Dao) SetLayer(ctx context.Context, in *authsession.TLAuthsessionSetLayer) error {
	_, _, err := d.execAuthData(
		ctx,
		in.GetAuthKeyId(),
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			return d.AuthsDAO.InsertOrUpdateLayer(
				ctx,
				&dataobject.AuthsDO{
					AuthKeyId:  in.GetAuthKeyId(),
					Layer:      in.GetLayer(),
					ClientIp:   in.GetIp(),
					DateActive: time.Now().Unix(),
				})
		})

	return err
}

func (d *Dao) SetInitConnection(ctx context.Context, i *authsession.TLAuthsessionSetInitConnection) error {
	_, _, err := d.execAuthData(
		ctx,
		i.GetAuthKeyId(),
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			do := &dataobject.AuthsDO{
				AuthKeyId:      i.GetAuthKeyId(),
				ApiId:          i.GetApiId(),
				DeviceModel:    i.GetDeviceModel(),
				SystemVersion:  i.GetSystemVersion(),
				AppVersion:     i.GetAppVersion(),
				SystemLangCode: i.GetSystemLangCode(),
				LangPack:       i.GetLangPack(),
				LangCode:       i.GetLangCode(),
				ClientIp:       i.GetIp(),
				Proxy:          i.GetProxy(),
				Params:         i.GetParams(),
				DateActive:     time.Now().Unix(),
			}
			if do.Params == "" {
				do.Params = "null"
			}
			return d.AuthsDAO.InsertOrUpdate(ctx, do)
		})

	return err
}

func (d *Dao) SetAndroidPushSessionId(ctx context.Context, userId, keyId, sessionId int64) error {
	_, _, err := d.execAuthData(
		ctx,
		keyId,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, err2 := d.AuthUsersDAO.UpdateAndroidPushSessionId(
				ctx,
				sessionId,
				keyId,
				userId)

			return 0, 0, err2
		})

	return err

}

func (d *Dao) GetCacheAuthData(ctx context.Context, authKeyId int64) (*CacheAuthData, error) {
	var (
		cData *CacheAuthData
	)

	// Sample the generation before the DB read. QueryRow writes the snapshot
	// under this key only. A bind or unbind that commits in between moves
	// the generation, and this write cannot land on the new slot.
	gen, err := d.authBindGen(ctx, authKeyId)
	if err != nil {
		return nil, err
	}

	err = d.CachedConn.QueryRow(
		ctx,
		&cData,
		genAuthDataCacheKey(authKeyId, gen),
		func(ctx context.Context, conn *sqlx.DB, v interface{}) error {
			cacheAuthData := &CacheAuthData{
				Client:   nil,
				BindUser: nil,
			}
			err := mr.Finish(
				func() error {
					do, err := d.AuthsDAO.SelectByAuthKeyId(ctx, authKeyId)
					if err != nil {
						return err
					}
					if do == nil {
						// No initConnection row yet. The auth_users binding
						// below is still valid; missing client info is not
						// "user not found".
						return nil
					}
					cacheAuthData.Client = authsession.MakeTLClientSession(&authsession.ClientSession{
						AuthKeyId:      authKeyId,
						Ip:             do.ClientIp,
						Layer:          do.Layer,
						ApiId:          do.ApiId,
						DeviceModel:    do.DeviceModel,
						SystemVersion:  do.SystemVersion,
						AppVersion:     do.AppVersion,
						SystemLangCode: do.SystemLangCode,
						LangPack:       do.LangPack,
						LangCode:       do.LangCode,
						Proxy:          do.Proxy,
						Params:         do.Params,
					}).To_ClientSession()

					return nil
				},
				func() error {
					do, _ := d.AuthUsersDAO.Select(ctx, authKeyId)
					if do != nil {
						cacheAuthData.BindUser = &BindUser{
							UserId:               do.UserId,
							Hash:                 do.Hash,
							DateCreated:          do.DateCreated,
							DateActivated:        do.DateActive,
							AndroidPushSessionId: do.AndroidPushSessionId,
							ActivityTrusted:      do.ActivityTrusted,
						}
					} else if d.authUserDeleted(ctx, authKeyId) {
						cacheAuthData.Revoked = true
					}
					return nil
				})

			if err != nil {
				return err
			}
			if cacheAuthData.Client == nil && cacheAuthData.BindUser == nil && !cacheAuthData.Revoked {
				return sqlc.ErrNotFound
			}
			*v.(**CacheAuthData) = cacheAuthData
			waitAuthLoadGate()

			return nil
		})
	if err != nil {
		return nil, err
	}

	return cData, nil
}
