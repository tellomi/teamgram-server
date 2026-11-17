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
	"net"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/geoip"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

// tellomiAPIID is the api_id Tellomi clients send in initConnection.
const tellomiAPIID int32 = 100001

// appNameForSession is authorization.app_name (account.getAuthorizations).
// Tellomi's api_id is labeled Tellomi. A stored lang_pack of "teamgram" is
// the same label and is rewritten. Other clients keep the lang_pack they sent.
func appNameForSession(apiID int32, langPack string) string {
	if apiID == tellomiAPIID || strings.EqualFold(langPack, "teamgram") {
		return "Tellomi"
	}
	return langPack
}

// normalizeClientIP accepts a host, host:port, [ipv6]:port, or the
// "ip|stamp" close marker and returns a parsed address.
func normalizeClientIP(ip string) net.IP {
	return geoip.ParseClientIP(ip)
}

func normalizedIPString(ip string) string {
	if parsed := normalizeClientIP(ip); parsed != nil {
		return parsed.String()
	}
	return ""
}

func (d *Dao) lookupCountryRegion(ip string) (string, string) {
	if d == nil || d.geo == nil {
		return "", ""
	}
	return d.geo.Lookup(ip)
}

// fillAuthorizationPlace keeps a resolved country. When GeoIP has nothing,
// a known IP is used as country so clients do not render the stub UNKNOWN.
func fillAuthorizationPlace(ip, country, region string) (string, string, string) {
	if country == "" && ip != "" {
		country = ip
	}
	return ip, country, region
}

// authorizationPlace is the ip/country/region triple for one session.
func (d *Dao) authorizationPlace(raw string) (string, string, string) {
	country, region := d.lookupCountryRegion(raw)
	return fillAuthorizationPlace(normalizedIPString(raw), country, region)
}

// authUserRef is one live auth_users row considered by reset.
type authUserRef struct {
	RowID     int64
	AuthKeyID int64
	Hash      int64
}

// freshResetWindowSeconds is the official current-session age.
// https://core.telegram.org/method/account.resetAuthorization
// https://core.telegram.org/method/auth.resetAuthorizations
// Less than 24 hours since THIS session logged on returns
// 406 FRESH_RESET_AUTHORISATION_FORBIDDEN. The target session's age and
// which device logged in first do not decide it.
const freshResetWindowSeconds int64 = 24 * 60 * 60

// freshResetForbidden reports the 24h gate for the actor session.
// A missing or zero date_created does not lock the account out forever.
// A created time in the future (clock skew) does not lock it out either.
// Elapsed time of zero is a session that just logged on and is forbidden.
// bindDateCreated is the created time stored when an auth key is bound.
// A positive stored time is an existing login and stays, so the 24h gate
// still measures that login. Reset writes date_created 0; the next bind of
// that key starts at now instead of skipping the gate.
func bindDateCreated(stored, now int64) int64 {
	if stored > 0 {
		return stored
	}
	if now > 0 {
		return now
	}
	return 0
}

func freshResetForbidden(createdUnix, nowUnix int64) bool {
	if createdUnix <= 0 || nowUnix < createdUnix {
		return false
	}
	return nowUnix-createdUnix < freshResetWindowSeconds
}

// ResetAuthorizationError is the decision applied before any auth_users row
// is soft-deleted. accountDeletion (actor auth key 0) skips the 24h gate.
// A non-zero hash that matches nothing is 400 HASH_INVALID, never success.
func ResetAuthorizationError(actorAuthKeyID, createdUnix, nowUnix, hash int64, matched bool) error {
	if actorAuthKeyID != 0 && freshResetForbidden(createdUnix, nowUnix) {
		return mtproto.ErrFreshResetAuthorisationForbidden
	}
	if hash != 0 && !matched {
		return mtproto.ErrHashInvalid
	}
	return nil
}

// UnionAuthKeyIDs keeps permanent and temporary key ids that must be told
// to drop. Zero ids are skipped. The caller is not added here.
func UnionAuthKeyIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// AuthorizationCreated reads auth_users.date_created for the actor's
// permanent key. found is false when that live row does not exist.
// A stored zero is found, so the caller can tell "unknown" from "missing".
func (d *Dao) AuthorizationCreated(ctx context.Context, authKeyId int64) (created int64, found bool, err error) {
	if d == nil || authKeyId == 0 {
		return 0, false, nil
	}
	var row struct {
		DateCreated int64 `db:"date_created"`
	}
	err = d.QueryRowPartial(ctx, &row, "select date_created from auth_users where auth_key_id = ? and deleted = 0 limit 1", authKeyId)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, false, nil
		}
		logx.WithContext(ctx).Errorf("AuthorizationCreated(%d) error: %v", authKeyId, err)
		return 0, false, err
	}
	return row.DateCreated, true, nil
}

// authUsersToReset selects rows to soft-delete.
// hash 0 drops every key except excludeAuthKeyID (0 excludes nobody, used
// when deleting the account). A non-zero hash drops that single other key.
func authUsersToReset(rows []authUserRef, excludeAuthKeyID, hash int64) (rowIDs, keyIDs []int64) {
	for _, v := range rows {
		if hash == 0 {
			if excludeAuthKeyID != 0 && excludeAuthKeyID == v.AuthKeyID {
				continue
			}
		} else if hash != v.Hash || excludeAuthKeyID == v.AuthKeyID {
			continue
		}
		rowIDs = append(rowIDs, v.RowID)
		keyIDs = append(keyIDs, v.AuthKeyID)
	}
	return rowIDs, keyIDs
}

// authUserDeleted reports a soft-deleted auth_users binding. A later RPC on
// that key must fail as logged out rather than as a fresh unauthorized key.
func (d *Dao) authUserDeleted(ctx context.Context, authKeyId int64) bool {
	var row struct {
		Id int64 `db:"id"`
	}
	err := d.QueryRowPartial(ctx, &row, "select id from auth_users where auth_key_id = ? and deleted = 1 limit 1", authKeyId)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) {
			logx.WithContext(ctx).Errorf("authUserDeleted(%d) error: %v", authKeyId, err)
		}
		return false
	}
	return row.Id != 0
}

func (d *Dao) GetAuthorization(ctx context.Context, authKeyId int64) (*mtproto.Authorization, error) {
	cData, err := d.GetCacheAuthData(ctx, authKeyId)
	if err != nil {
		return nil, err
	}

	ip, country, region := d.authorizationPlace(cData.ClientIp())

	authorization := mtproto.MakeTLAuthorization(&mtproto.Authorization{
		Current:         false,
		OfficialApp:     true,
		Hash:            0,
		PasswordPending: false,
		DeviceModel:     cData.DeviceModel(),
		Platform:        "",
		SystemVersion:   cData.SystemVersion(),
		ApiId:           cData.ApiId(),
		AppName:         appNameForSession(cData.ApiId(), cData.LangPack()),
		AppVersion:      cData.AppVersion(),
		DateCreated:     0,
		DateActive:      0,
		Ip:              ip,
		Country:         country,
		Region:          region,
	}).To_Authorization()
	d.applyAuthorizationKeyFlags(ctx, authKeyId, authorization)
	return authorization, nil
}

type idxId struct {
	idx         int
	id          int64
	hash        int64
	useHash     bool
	encrypted   bool
	calls       bool
	unconfirmed bool
}

func (d *Dao) GetAuthorizations(ctx context.Context, userId int64, excludeAuthKeyId int64) (authorizations []*mtproto.Authorization) {
	d.ExpireStaleAuthorizations(ctx, userId, excludeAuthKeyId, time.Now().Unix())
	doList, _ := d.AuthUsersDAO.SelectListByUserId(ctx, userId)
	if len(doList) == 0 {
		return
	}
	flags := d.authorizationSettingsForUser(ctx, userId, time.Now().Unix())

	authorizations = make([]*mtproto.Authorization, len(doList)+1)
	mr.ForEach(
		func(source chan<- interface{}) {
			for i := 0; i < len(doList); i++ {
				item := idxId{idx: i, id: doList[i].AuthKeyId}
				if row, ok := flags[item.id]; ok {
					item.useHash = row.Hash != 0
					item.hash = row.Hash
					item.encrypted = row.EncryptedRequestsDisabled
					item.calls = row.CallRequestsDisabled
					item.unconfirmed = row.Unconfirmed
				}
				source <- item
			}
		},
		func(item interface{}) {
			idx := item.(idxId)
			//kId := item.(int64)
			cData, _ := d.GetCacheAuthData(ctx, idx.id)
			if cData != nil {
				ip, country, region := d.authorizationPlace(cData.ClientIp())
				hash := cData.Hash()
				// The hash account.changeAuthorizationSettings matches is the
				// auth_users.hash column. The cache copy can lag a re-bind.
				if idx.useHash {
					hash = idx.hash
				}
				authorization := mtproto.MakeTLAuthorization(&mtproto.Authorization{
					Current:                   false,
					OfficialApp:               true,
					PasswordPending:           false,
					Hash:                      hash,
					EncryptedRequestsDisabled: idx.encrypted,
					CallRequestsDisabled:      idx.calls,
					Unconfirmed:               idx.unconfirmed,
					DeviceModel:               cData.DeviceModel(),
					Platform:                  "",
					SystemVersion:             cData.SystemVersion(),
					ApiId:                     cData.ApiId(),
					AppName:                   appNameForSession(cData.ApiId(), cData.LangPack()),
					AppVersion:                cData.AppVersion(),
					DateCreated:               int32(cData.DateCreated()),
					DateActive:                int32(cData.DateActivated()),
					Ip:                        ip,
					Country:                   country,
					Region:                    region,
				}).To_Authorization()

				if idx.id == excludeAuthKeyId {
					authorization.Current = true
					authorization.Hash = 0
					authorizations[0] = authorization
				} else {
					authorizations[idx.idx+1] = authorization
				}
			}
		})

	return removeAllNil(authorizations)
}

func (d *Dao) ResetAuthorization(ctx context.Context, userId int64, authKeyId, hash int64) []int64 {
	var (
		hashList  []int64
		keyIdList []int64
	)

	var refs []authUserRef
	_, _ = d.AuthUsersDAO.SelectListByUserIdWithCB(
		ctx,
		userId,
		func(sz, i int, v *dataobject.AuthUsersDO) {
			refs = append(refs, authUserRef{RowID: v.Id, AuthKeyID: v.AuthKeyId, Hash: v.Hash})
		})
	hashList, keyIdList = authUsersToReset(refs, authKeyId, hash)
	if len(keyIdList) == 0 {
		return keyIdList
	}

	result := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := lockAuthIdentities(tx, keyIdList); err != nil {
			result.Err = err
			return
		}
		_, err := d.AuthUsersDAO.DeleteByHashListTx(tx, hashList)
		result.Err = err
	})
	if result.Err != nil {
		logx.WithContext(ctx).Errorf("resetAuthorization user_id=%d: %v", userId, result.Err)
		return keyIdList
	}
	if err := d.bumpAuthBindGens(ctx, keyIdList); err != nil {
		logx.WithContext(ctx).Errorf("resetAuthorization cache user_id=%d: %v", userId, err)
	}
	d.deleteDeviceTokens(ctx, userId, keyIdList)

	return keyIdList
}
