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
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// InboxUnpinAllMessages
// inbox.unpinAllMessages user_id:long auth_key_id:long peer_type:int peer_id:long = Void;
func (c *InboxCore) InboxUnpinAllMessages(in *inbox.TLInboxUnpinAllMessages) (*mtproto.Void, error) {
	peer := mtproto.MakePeerUtil(in.PeerType, in.PeerId)

	switch peer.PeerType {
	case mtproto.PEER_USER:
		dialogID := mtproto.MakeDialogId(peer.PeerId, peer.PeerType, in.UserId)
		c.clearPinnedBox(peer.PeerId, dialogID.A, dialogID.B, mtproto.PEER_USER, in.UserId, mtproto.MakePeerUser(in.UserId))
	case mtproto.PEER_CHAT:
		dialogID := mtproto.MakeDialogId(0, peer.PeerType, in.PeerId)
		_, _ = c.svcCtx.Dao.ChatParticipantsDAO.SelectListWithCB(
			c.ctx,
			peer.PeerId,
			func(sz, i int, v *dataobject.ChatParticipantsDO) {
				if v.UserId == in.UserId || v.State != mtproto.ChatMemberStateNormal {
					return
				}
				c.clearPinnedBox(v.UserId, dialogID.A, dialogID.B, mtproto.PEER_CHAT, peer.PeerId, mtproto.MakePeerChat(peer.PeerId))
			},
		)
	case mtproto.PEER_CHANNEL:
	}

	return mtproto.EmptyVoid, nil
}

// clearPinnedBox drops pinned on one user's copy and tells that session.
// The update used to go out while the rows stayed pinned=1.
func (c *InboxCore) clearPinnedBox(userID, dialogA, dialogB int64, peerType int32, peerID int64, updatePeer *mtproto.Peer) {
	var ids []int32
	_, _ = c.svcCtx.Dao.MessagesDAO.SelectPinnedListWithCB(
		c.ctx,
		userID,
		dialogA,
		dialogB,
		func(sz, i int, v *dataobject.MessagesDO) {
			ids = append(ids, v.UserMessageBoxId)
		})
	if len(ids) == 0 {
		return
	}
	if _, err := c.svcCtx.Dao.MessagesDAO.UpdateUnPinnedByIdList(c.ctx, userID, ids); err != nil {
		c.Logger.Errorf("inbox.unpinAllMessages - error: %v", err)
	}
	if c.svcCtx.Dao.DialogClient != nil {
		_, _ = c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialog(c.ctx, &dialog.TLDialogInsertOrUpdateDialog{
			UserId:      userID,
			PeerType:    peerType,
			PeerId:      peerID,
			PinnedMsgId: &wrapperspb.Int32Value{Value: 0},
		})
	}
	pts := c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, userID, len(ids))
	ptsCount := int32(len(ids))
	updatePinnedMessages := mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{
		Pinned:    false,
		Peer_PEER: updatePeer,
		Messages:  ids,
		Pts_INT32: pts,
		PtsCount:  ptsCount,
	}).To_Update()
	c.persistPtsUpdate(c.ctx, userID, updatePinnedMessages)
	_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId:  userID,
		Updates: mtproto.MakeUpdatesByUpdates(updatePinnedMessages),
	})
}
