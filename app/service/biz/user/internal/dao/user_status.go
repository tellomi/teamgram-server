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
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	userPresencesKeyPrefix = "user_presences"
)

func genUserPresencesKey(userId int64) string {
	return fmt.Sprintf("%s_%d", userPresencesKeyPrefix, userId)
}

func isUserPresencesKey(k string) bool {
	return strings.HasPrefix(k, userPresencesKeyPrefix)
}

func parseUserPresencesKey(k string) int64 {
	if strings.HasPrefix(k, userPresencesKeyPrefix+"_") {
		v, _ := strconv.ParseInt(k[len(userPresencesKeyPrefix)+1:], 10, 64)
		return v
	}

	return 0
}

func (d *Dao) GetLastSeenAt(ctx context.Context, id int64) (*dataobject.UserPresencesDO, error) {
	var (
		do = &dataobject.UserPresencesDO{}
	)

	err := d.CachedConn.QueryRow(
		ctx,
		do,
		genUserPresencesKey(id),
		func(ctx context.Context, conn *sqlx.DB, v interface{}) error {
			do2, err := d.UserPresencesDAO.Select(ctx, id)
			if err != nil {
				return err
			}
			if do2 != nil {
				*v.(*dataobject.UserPresencesDO) = *do2
			} else {
				return sqlc.ErrNotFound
			}
			return nil
		})
	if err != nil {
		if errors.Is(err, sqlc.ErrNotFound) && id > 0 {
			// A user created before presence rows were written, or a test
			// account that never called account.updateStatus, has no row.
			// Insert the missing row only. An existing heartbeat is not updated.
			d.insertPresenceIfAbsent(ctx, id)
		}
		return nil, err
	}

	return do, nil
}

func presenceCursor(row *dataobject.UserPresencesDO) int64 {
	if row == nil {
		return 0
	}
	return mtproto.PresenceCursor(row.LastSeenAt, row.Expires)
}

func (d *Dao) insertPresenceIfAbsent(ctx context.Context, userId int64) {
	if d == nil || d.DB == nil || userId <= 0 {
		return
	}
	_, err := d.DB.Exec(ctx, "insert ignore into user_presences(user_id, last_seen_at, expires) values (?, 0, 0)", userId)
	if err != nil {
		logx.WithContext(ctx).Errorf("insertPresenceIfAbsent - user_id=%d error: %v", userId, err)
	}
}

func (d *Dao) PutLastSeenAt(ctx context.Context, userId int64, lastSeenAt int64, expires int32) {
	do := &dataobject.UserPresencesDO{
		UserId:     userId,
		LastSeenAt: lastSeenAt,
		Expires:    expires,
	}

	d.CachedConn.SetCache(ctx, genUserPresencesKey(userId), do)
	if _, _, err := d.UserPresencesDAO.InsertOrUpdate(ctx, do); err != nil {
		logx.WithContext(ctx).Errorf("PutLastSeenAt - user_id=%d error: %v", userId, err)
	}
}
