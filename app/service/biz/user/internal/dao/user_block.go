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
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

var (
	contactsBlockPeerPrefix = "user_block_peer"
)

type CachedPeerBlocked struct {
	PeerBlocked *mtproto.PeerBlocked `json:"peer_blocked"`
}

func (c *CachedPeerBlocked) IsEmpty() bool {
	if c == nil {
		return true
	}

	return c.PeerBlocked == nil
}

func genContactsBlockPeerCacheKey(id, blockedId int64) string {
	return fmt.Sprintf("%s_%d_%d", contactsBlockPeerPrefix, id, blockedId)
}

// BlockedEitherWay returns ids from the candidate list that this user
// blocked or that have blocked this user. Both directions are one query.
func (d *Dao) BlockedEitherWay(ctx context.Context, self int64, ids []int64) ([]int64, error) {
	if d == nil || self == 0 || len(ids) == 0 {
		return []int64{}, nil
	}
	mine, err := d.UserPeerBlocksDAO.SelectListByIdList(ctx, self, ids)
	if err != nil {
		return nil, err
	}
	var theirs []int64
	query := fmt.Sprintf("select user_id from user_peer_blocks where peer_type = ? and peer_id = ? and deleted = 0 and user_id in (%s)", sqlx.InInt64List(ids))
	if err = d.QueryRowsPartial(ctx, &theirs, query, mtproto.PEER_USER, self); err != nil && !errors.Is(err, sqlx.ErrNotFound) {
		return nil, err
	}
	seen := make(map[int64]struct{}, len(mine)+len(theirs))
	out := make([]int64, 0, len(mine)+len(theirs))
	for _, id := range append(mine, theirs...) {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func (d *Dao) CheckBlocked(ctx context.Context, id, blockedId int64) bool {
	var (
		blocked = new(CachedPeerBlocked)
	)
	d.CachedConn.QueryRow(
		ctx,
		blocked,
		genContactsBlockPeerCacheKey(id, blockedId),
		func(ctx context.Context, conn *sqlx.DB, v interface{}) error {
			do, err := d.UserPeerBlocksDAO.Select(ctx, id, mtproto.PEER_USER, blockedId)
			if err != nil {
				return err
			}
			if do != nil {
				v.(*CachedPeerBlocked).PeerBlocked = mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
					PeerId: mtproto.MakePeerUser(do.PeerId),
					Date:   int32(do.Date),
				}).To_PeerBlocked()
			} else {
				return sqlc.ErrNotFound
			}

			return nil
		},
	)

	return !blocked.IsEmpty()
}

func (d *Dao) BlockUser(ctx context.Context, id, blockId int64) bool {
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			return d.UserPeerBlocksDAO.InsertOrUpdate(ctx, &dataobject.UserPeerBlocksDO{
				UserId:   id,
				PeerType: mtproto.PEER_USER,
				PeerId:   blockId,
				Date:     time.Now().Unix(),
			})
		},
		genContactsBlockPeerCacheKey(id, blockId))

	return err == nil
}

func (d *Dao) UnBlockUser(ctx context.Context, id, unblockId int64) bool {
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			affected, err := d.UserPeerBlocksDAO.Delete(
				ctx,
				id,
				mtproto.PEER_USER,
				unblockId)
			return 0, affected, err
		},
		genContactsBlockPeerCacheKey(id, unblockId))

	return err == nil
}
