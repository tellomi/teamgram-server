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

	"github.com/zeromicro/go-zero/core/logx"
)

// AdvanceSavedMessagesTop moves the Saved Messages dialog forward.
// Saved Messages is the dialog with user_id = peer_id, stored as peer_type 2
// (inputPeerSelf is sent as a user peer) or peer_type 1. A send addressed at
// one of those types must still update the row that was stored as the other.
// top_message only increases, so an older message cannot rewind the dialog.
func (dao *DialogsDAO) AdvanceSavedMessagesTop(ctx context.Context, topMessage int32, date2 int64, userId int64) (rowsAffected int64, err error) {
	query := "update dialogs set top_message = ?, date2 = ? where user_id = ? and peer_id = ? and peer_type in (1, 2) and top_message < ?"
	r, err := dao.db.Exec(ctx, query, topMessage, date2, userId, userId, topMessage)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in AdvanceSavedMessagesTop(_), error: %v", err)
		return
	}
	rowsAffected, err = r.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in AdvanceSavedMessagesTop(_), error: %v", err)
	}
	return
}
