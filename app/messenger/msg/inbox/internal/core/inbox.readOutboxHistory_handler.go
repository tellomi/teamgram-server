// Copyright 2024 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// skipPrivateSelfOutbox is true when a private-chat read would mark the
// reader's own outbox. Saved Messages is a private dialog with yourself and
// is not a read receipt.
func skipPrivateSelfOutbox(peerType int32, userId, peerId int64) bool {
	switch peerType {
	case mtproto.PEER_USER, mtproto.PEER_SELF:
		return userId == peerId
	default:
		return false
	}
}

// InboxReadOutboxHistory
// inbox.readOutboxHistory user_id:long peer_type:int peer_id:long max_dialog_message_id:int64 = Void;
func (c *InboxCore) InboxReadOutboxHistory(in *inbox.TLInboxReadOutboxHistory) (*mtproto.Void, error) {
	// Reading one's own latest message used to call this with the reader as
	// both the outbox owner and the peer, which marked that outbox read.
	if skipPrivateSelfOutbox(in.PeerType, in.UserId, in.PeerId) {
		return mtproto.EmptyVoid, nil
	}

	switch in.PeerType {
	case mtproto.PEER_USER:
		replyId, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageDataId(
			c.ctx,
			in.UserId,
			in.MaxDialogMessageId)
		if err != nil {
			c.Logger.Errorf("inbox.readOutboxHistory - error: %v", err)
			return nil, err
		} else if replyId == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("inbox.readOutboxHistory - error: %v", err)
			return nil, err
		} else if replyId.SenderUserId != in.UserId {
			// The row is not this user's outgoing message. Do not move their outbox.
			return mtproto.EmptyVoid, nil
		}
		c.Logger.Infof("inbox.readOutboxHistory: %v", replyId)

		// The reader is the private-chat peer. A failed insert must not skip
		// the dialog update: checkmarks still move, and the date write is
		// best-effort.
		c.rememberOutboxRead(
			in.UserId,
			in.PeerId,
			mtproto.MakePeerDialogId(in.PeerType, in.PeerId),
			replyId.UserMessageBoxId)

		_, _ = c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialog(
			c.ctx,
			&dialog.TLDialogInsertOrUpdateDialog{
				UserId:          in.UserId,
				PeerType:        in.PeerType,
				PeerId:          in.PeerId,
				TopMessage:      nil,
				ReadOutboxMaxId: &wrapperspb.Int32Value{Value: replyId.UserMessageBoxId},
				ReadInboxMaxId:  nil,
				UnreadCount:     nil,
				UnreadMark:      false,
				PinnedMsgId:     nil,
				Date2:           nil,
			})
		c.Logger.Infof("inbox.readOutboxHistory: (%d, %d, %d)",
			replyId.UserMessageBoxId,
			in.UserId,
			mtproto.MakePeerDialogId(in.PeerType, in.PeerId))

		updateReadHistoryOutbox := mtproto.MakeTLUpdateReadHistoryOutbox(&mtproto.Update{
			Peer_PEER: mtproto.MakePeerUser(in.PeerId),
			MaxId:     replyId.UserMessageBoxId,
			Pts_INT32: c.svcCtx.Dao.IDGenClient2.NextPtsId(c.ctx, in.UserId),
			PtsCount:  1,
		}).To_Update()
		c.persistPtsUpdate(c.ctx, in.UserId, updateReadHistoryOutbox)

		_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  in.UserId,
			Updates: mtproto.MakeUpdatesByUpdates(updateReadHistoryOutbox),
		})
	case mtproto.PEER_CHAT:
		replyId, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageDataId(
			c.ctx,
			in.UserId,
			in.MaxDialogMessageId)
		if err != nil {
			c.Logger.Errorf("inbox.readOutboxHistory - error: %v", err)
			return nil, err
		} else if replyId == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("inbox.readOutboxHistory - error: %v", err)
			return nil, err
		} else if replyId.SenderUserId != in.UserId {
			return mtproto.EmptyVoid, nil
		}
		c.Logger.Infof("inbox.readOutboxHistory: %v", replyId)

		_, _ = c.svcCtx.Dao.DialogsDAO.SelectPeerDialogListWithCB(
			c.ctx,
			replyId.UserId,
			[]int64{mtproto.MakePeerDialogId(in.PeerType, in.PeerId)},
			func(sz, i int, v *dataobject.DialogsDO) {
				if v.ReadOutboxMaxId < replyId.UserMessageBoxId {
					_, _ = c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialog(
						c.ctx,
						&dialog.TLDialogInsertOrUpdateDialog{
							UserId:          replyId.UserId,
							PeerType:        in.PeerType,
							PeerId:          in.PeerId,
							TopMessage:      nil,
							ReadOutboxMaxId: &wrapperspb.Int32Value{Value: replyId.UserMessageBoxId},
							ReadInboxMaxId:  nil,
							UnreadCount:     nil,
							UnreadMark:      false,
							PinnedMsgId:     nil,
							Date2:           nil,
						})
					c.Logger.Infof("inbox.updateHistoryReaded: (%d, %d, %d)",
						replyId.UserMessageBoxId,
						replyId.PeerId,
						mtproto.MakePeerDialogId(in.PeerType, in.PeerId))

					updateReadHistoryOutbox := mtproto.MakeTLUpdateReadHistoryOutbox(&mtproto.Update{
						Peer_PEER: mtproto.MakePeerChat(in.PeerId),
						MaxId:     replyId.UserMessageBoxId,
						Pts_INT32: c.svcCtx.Dao.IDGenClient2.NextPtsId(c.ctx, in.UserId),
						PtsCount:  1,
					}).To_Update()
					c.persistPtsUpdate(c.ctx, in.UserId, updateReadHistoryOutbox)

					_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
						UserId:  in.UserId,
						Updates: mtproto.MakeUpdatesByUpdates(updateReadHistoryOutbox),
					})
				}
			},
		)
	case mtproto.PEER_CHANNEL:
		// TODO
	}

	return mtproto.EmptyVoid, nil
}
