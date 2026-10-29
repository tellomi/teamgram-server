// Copyright 2026 Teamgram Authors
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

package mysql_dao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// ErrMessageBoxCollision means (user_id, user_message_box_id) already stores
// a different message. Callers must fail the send. Keeping the old row made
// the Redis id reset drop the new messages.
var ErrMessageBoxCollision = errors.New("message box id already holds a different message")

const messageInsertSQL = "insert into __TABLE__(user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, saved_peer_type, saved_peer_id, date2, ttl_period) values (:user_id, :user_message_box_id, :dialog_id1, :dialog_id2, :dialog_message_id, :sender_user_id, :peer_type, :peer_id, :random_id, :message_filter_type, :message_data, :message, :mentioned, :media_unread, :pinned, :saved_peer_type, :saved_peer_id, :date2, :ttl_period)"

const selectBoxSQL = "select id, user_id, user_message_box_id, sender_user_id, peer_type, peer_id, random_id, message, date2 from __TABLE__ where user_id = ? and user_message_box_id = ? limit 1"

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return true
	}
	// Some wrappers only keep the driver text.
	msg := err.Error()
	return strings.Contains(msg, "Error 1062") || strings.Contains(msg, "Duplicate entry")
}

// sameStoredMessage is true only for a retry of the same send.
// A different random id, or the same box id with a different date or body,
// is a collision caused by the sequence counter moving backwards.
func sameStoredMessage(existing, incoming *dataobject.MessagesDO) bool {
	if existing == nil || incoming == nil {
		return false
	}
	if existing.UserId != incoming.UserId || existing.UserMessageBoxId != incoming.UserMessageBoxId {
		return false
	}
	if incoming.RandomId != 0 || existing.RandomId != 0 {
		return existing.RandomId == incoming.RandomId && existing.SenderUserId == incoming.SenderUserId
	}
	return existing.SenderUserId == incoming.SenderUserId &&
		existing.Date2 == incoming.Date2 &&
		existing.Message == incoming.Message &&
		existing.PeerType == incoming.PeerType &&
		existing.PeerId == incoming.PeerId
}

func (dao *MessagesDAO) replayOrCollision(ctx context.Context, tx *sqlx.Tx, do *dataobject.MessagesDO) (int64, int64, error) {
	existing, err := dao.selectBox(ctx, tx, do.UserId, do.UserMessageBoxId)
	if err != nil {
		return 0, 0, err
	}
	if sameStoredMessage(existing, do) {
		logx.WithContext(ctx).Infof("message insert replay user_id=%d box_id=%d id=%d", do.UserId, do.UserMessageBoxId, existing.Id)
		return existing.Id, 0, nil
	}
	var existDate int64
	var existMsg string
	var existRandom int64
	if existing != nil {
		existDate = existing.Date2
		existMsg = existing.Message
		existRandom = existing.RandomId
	}
	logx.WithContext(ctx).Errorf("message box id collision: refused to overwrite user_id=%d box_id=%d existing_date=%d existing_random=%d existing_message=%q incoming_date=%d incoming_random=%d incoming_message=%q",
		do.UserId, do.UserMessageBoxId, existDate, existRandom, clip(existMsg), do.Date2, do.RandomId, clip(do.Message))
	return 0, 0, fmt.Errorf("%w: user_id=%d box_id=%d existing_date=%d incoming_date=%d",
		ErrMessageBoxCollision, do.UserId, do.UserMessageBoxId, existDate, do.Date2)
}

func (dao *MessagesDAO) selectBox(ctx context.Context, tx *sqlx.Tx, userId int64, boxId int32) (*dataobject.MessagesDO, error) {
	query := strings.Replace(selectBoxSQL, "__TABLE__", dao.CalcTableName(userId), -1)
	do := &dataobject.MessagesDO{}
	var err error
	if tx != nil {
		err = tx.QueryRowPartial(do, query, userId, boxId)
	} else {
		err = dao.db.QueryRowPartial(ctx, do, query, userId, boxId)
	}
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return do, nil
}

func clip(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
