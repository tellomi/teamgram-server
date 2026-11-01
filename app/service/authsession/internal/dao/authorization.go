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

	return mtproto.MakeTLAuthorization(&mtproto.Authorization{
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
	}).To_Authorization(), nil
}

type idxId struct {
	idx int
	id  int64
}

func (d *Dao) GetAuthorizations(ctx context.Context, userId int64, excludeAuthKeyId int64) (authorizations []*mtproto.Authorization) {
	doList, _ := d.AuthUsersDAO.SelectListByUserId(ctx, userId)
	if len(doList) == 0 {
		return
	}

	authorizations = make([]*mtproto.Authorization, len(doList)+1)
	mr.ForEach(
		func(source chan<- interface{}) {
			for i := 0; i < len(doList); i++ {
				source <- idxId{i, doList[i].AuthKeyId}
			}
		},
		func(item interface{}) {
			idx := item.(idxId)
			//kId := item.(int64)
			cData, _ := d.GetCacheAuthData(ctx, idx.id)
			if cData != nil {
				ip, country, region := d.authorizationPlace(cData.ClientIp())
				authorization := mtproto.MakeTLAuthorization(&mtproto.Authorization{
					Current:         false,
					OfficialApp:     true,
					PasswordPending: false,
					Hash:            cData.Hash(),
					DeviceModel:     cData.DeviceModel(),
					Platform:        "",
					SystemVersion:   cData.SystemVersion(),
					ApiId:           cData.ApiId(),
					AppName:         appNameForSession(cData.ApiId(), cData.LangPack()),
					AppVersion:      cData.AppVersion(),
					DateCreated:     int32(cData.DateCreated()),
					DateActive:      int32(cData.DateActivated()),
					Ip:              ip,
					Country:         country,
					Region:          region,
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
		cacheKeyIdList []string
		hashList       []int64
		keyIdList      []int64
	)

	var refs []authUserRef
	_, _ = d.AuthUsersDAO.SelectListByUserIdWithCB(
		ctx,
		userId,
		func(sz, i int, v *dataobject.AuthUsersDO) {
			refs = append(refs, authUserRef{RowID: v.Id, AuthKeyID: v.AuthKeyId, Hash: v.Hash})
		})
	hashList, keyIdList = authUsersToReset(refs, authKeyId, hash)
	for _, id := range keyIdList {
		cacheKeyIdList = append(cacheKeyIdList, genAuthDataCacheKey(id))
	}
	if len(keyIdList) == 0 {
		return keyIdList
	}

	_, _, _ = d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, _ = d.AuthUsersDAO.DeleteByHashList(ctx, hashList)
			return 0, 0, nil
		},
		cacheKeyIdList...)

	return keyIdList
}
