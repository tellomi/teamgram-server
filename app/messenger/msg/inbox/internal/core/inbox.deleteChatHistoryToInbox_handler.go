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

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/deletehistory"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
)

// InboxDeleteChatHistoryToInbox
// inbox.deleteChatHistoryToInbox from_id:long peer_chat_id:long max_id:int = Void;
func (c *InboxCore) InboxDeleteChatHistoryToInbox(in *inbox.TLInboxDeleteChatHistoryToInbox) (*mtproto.Void, error) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_CHAT, in.PeerChatId)
	// max_id is the caller's box id. This user's own ids are unrelated, so
	// the page walks every live message in the chat dialog.
	q := deletehistory.Query{}
	for {
		rows, err := c.svcCtx.Dao.ListDialogDeletePage(c.ctx, in.FromId, peer, deletehistory.PageSelect(q, 0, deletehistory.Batch+1), false)
		if err != nil {
			c.Logger.Errorf("inbox.deleteChatHistoryToInbox - error: %v", err)
			return nil, err
		}
		page, offset := deletehistory.Page(inboxRows(rows), deletehistory.Batch)
		if len(page) == 0 {
			return mtproto.EmptyVoid, nil
		}
		ids := inboxIDs(page)
		if _, err = c.svcCtx.Dao.DeleteByMessageIdList(c.ctx, in.FromId, ids); err != nil {
			c.Logger.Errorf("inbox.deleteChatHistoryToInbox - error: %v", err)
			return nil, err
		}
		live, err := c.svcCtx.Dao.LiveMessageBoxIDs(c.ctx, in.FromId, ids)
		if err != nil {
			return nil, err
		}
		deleted := make([]int32, 0, len(ids))
		for _, id := range ids {
			if _, still := live[id]; !still {
				deleted = append(deleted, id)
			}
		}
		if len(deleted) == 0 {
			c.Logger.Errorf("inbox.deleteChatHistoryToInbox - deleted none of %d", len(ids))
			return nil, mtproto.ErrInternalServerError
		}
		if err = c.pushInboxDelete(in.FromId, peer, deleted, deleted[0]); err != nil {
			return nil, err
		}
		if offset == 0 {
			return mtproto.EmptyVoid, nil
		}
		if len(deleted) < len(ids) {
			return nil, mtproto.ErrInternalServerError
		}
	}
}
