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

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// SelectNewestIncoming
// select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id
// from messages where user_id = :user_id and dialog_id1 = :dialog_id1 and dialog_id2 = :dialog_id2
// and sender_user_id <> :exclude_sender and user_message_box_id <= :max_box_id and deleted = 0
// order by user_message_box_id desc limit 1
func (dao *MessagesDAO) SelectNewestIncoming(ctx context.Context, userId int64, dialogId1 int64, dialogId2 int64, excludeSender int64, maxBoxId int32) (rValue *dataobject.MessagesDO, err error) {
	var (
		query = "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id from " + dao.CalcTableName(userId) + " where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and sender_user_id <> ? and user_message_box_id <= ? and deleted = 0 order by user_message_box_id desc limit 1"
		do    = &dataobject.MessagesDO{}
	)

	err = dao.db.QueryRowPartial(ctx, do, query, userId, dialogId1, dialogId2, excludeSender, maxBoxId)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) {
			logx.WithContext(ctx).Errorf("queryx in SelectNewestIncoming(_), error: %v", err)
			return
		}
		err = nil
		return
	}
	rValue = do
	return
}
