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
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// InboxDeleteUserHistoryToInbox
// inbox.deleteUserHistoryToInbox flags:# from_id:long peer_user_id:long just_clear:flags.1?true max_id:int = Void;
//
// max_id on this request is the caller's box id, not the peer's, so it is
// not applied here. The caller revokes one page by dialog_message_id. This
// method clears whatever of that dialog is still in the peer's box, one
// page at a time, and returns the error when a page does not delete.
func (c *InboxCore) InboxDeleteUserHistoryToInbox(in *inbox.TLInboxDeleteUserHistoryToInbox) (*mtproto.Void, error) {
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, in.FromId)
	q := deletehistory.Query{JustClear: in.GetJustClear()}
	var anchorID int32
	var anchor *mtproto.Message
	if q.JustClear {
		top, err := c.svcCtx.Dao.ListDialogDeletePage(c.ctx, in.PeerUserId, peer, deletehistory.PageSelect(q, 0, 1), true)
		if err != nil {
			c.Logger.Errorf("inbox.deleteUserHistoryToInbox - error: %v", err)
			return nil, err
		}
		if len(top) > 0 {
			anchorID = top[0].UserMessageBoxId
			anchor = inboxMessage(&top[0])
		}
	}
	for {
		rows, err := c.svcCtx.Dao.ListDialogDeletePage(c.ctx, in.PeerUserId, peer, deletehistory.PageSelect(q, anchorID, deletehistory.Batch+1), false)
		if err != nil {
			c.Logger.Errorf("inbox.deleteUserHistoryToInbox - error: %v", err)
			return nil, err
		}
		page, offset := deletehistory.Page(inboxRows(rows), deletehistory.Batch)
		if len(page) == 0 {
			break
		}
		ids := inboxIDs(page)
		if _, err = c.svcCtx.Dao.DeleteByMessageIdList(c.ctx, in.PeerUserId, ids); err != nil {
			c.Logger.Errorf("inbox.deleteUserHistoryToInbox - error: %v", err)
			return nil, err
		}
		live, err := c.svcCtx.Dao.LiveMessageBoxIDs(c.ctx, in.PeerUserId, ids)
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
			c.Logger.Errorf("inbox.deleteUserHistoryToInbox - deleted none of %d", len(ids))
			return nil, mtproto.ErrInternalServerError
		}
		if err = c.pushInboxDelete(in.PeerUserId, peer, deleted, deleted[0]); err != nil {
			return nil, err
		}
		if offset == 0 || len(deleted) < len(ids) {
			if len(deleted) < len(ids) {
				return nil, mtproto.ErrInternalServerError
			}
			break
		}
	}
	if q.JustClear && anchor != nil && !deletehistory.IsHistoryClear(anchor) {
		cleared := mtproto.MakeTLMessageService(&mtproto.Message{
			Out:    true,
			Id:     anchor.GetId(),
			FromId: mtproto.MakePeerUser(in.PeerUserId),
			PeerId: mtproto.MakePeerUser(in.FromId),
			Date:   anchor.GetDate(),
			Action: mtproto.MakeTLMessageActionHistoryClear(nil).To_MessageAction(),
		}).To_Message()
		if _, err := c.svcCtx.Dao.EditUserOutboxMessage(c.ctx, in.PeerUserId, in.FromId, cleared); err != nil {
			c.Logger.Errorf("inbox.deleteUserHistoryToInbox - error: %v", err)
			return nil, err
		}
		pts := c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, in.PeerUserId, 1)
		upd := mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
			Message_MESSAGE: cleared,
			Pts_INT32:       pts,
			PtsCount:        1,
		}).To_Update()
		c.persistPtsUpdate(c.ctx, in.PeerUserId, upd)
		_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  in.PeerUserId,
			Updates: mtproto.MakeUpdatesByUpdates(upd),
		})
	}
	return mtproto.EmptyVoid, nil
}

func (c *InboxCore) pushInboxDelete(userID int64, peer *mtproto.PeerUtil, ids []int32, maxID int32) error {
	pts := c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, userID, len(ids)+1)
	del := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
		Messages:  ids,
		Pts_INT32: pts - 1,
		PtsCount:  int32(len(ids)),
	}).To_Update()
	read := mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
		Peer_PEER: peer.ToPeer(),
		MaxId:     maxID,
		Pts_INT32: pts,
		PtsCount:  1,
	}).To_Update()
	c.persistPtsUpdate(c.ctx, userID, del)
	c.persistPtsUpdate(c.ctx, userID, read)
	_, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId:  userID,
		Updates: mtproto.MakeUpdatesByUpdates(del, read),
	})
	return err
}

func inboxRows(rows []dataobject.MessagesDO) []deletehistory.Row {
	out := make([]deletehistory.Row, 0, len(rows))
	for i := range rows {
		out = append(out, deletehistory.Row{ID: rows[i].UserMessageBoxId, Date: rows[i].Date2})
	}
	return out
}

func inboxIDs(rows []deletehistory.Row) []int32 {
	ids := make([]int32, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	return ids
}

func inboxMessage(row *dataobject.MessagesDO) *mtproto.Message {
	if row == nil || row.MessageData == "" {
		return nil
	}
	m := new(mtproto.Message)
	if err := jsonx.UnmarshalFromString(row.MessageData, m); err != nil {
		return nil
	}
	return m.FixData()
}
