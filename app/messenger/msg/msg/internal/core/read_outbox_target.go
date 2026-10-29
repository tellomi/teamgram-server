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

package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// chooseOutboxReadMessage picks the message whose sender should see a read
// receipt. The message at max_id is often the reader's own; marking that
// sender's outbox read draws double ticks before anyone else has read it.
// newestOther is the newest message at or below max_id from somebody else.
func chooseOutboxReadMessage(readerUserId int64, atMax, newestOther *dataobject.MessagesDO) *dataobject.MessagesDO {
	if atMax == nil || atMax.SenderUserId == 0 {
		return nil
	}
	if atMax.SenderUserId != readerUserId {
		return atMax
	}
	if newestOther == nil || newestOther.SenderUserId == 0 || newestOther.SenderUserId == readerUserId {
		return nil
	}
	return newestOther
}

func (c *MsgCore) markPeerOutboxRead(ctx context.Context, in *msg.TLMsgReadHistoryV2, maxId int32, atMax *dataobject.MessagesDO) {
	var newestOther *dataobject.MessagesDO
	if atMax != nil && atMax.SenderUserId == in.UserId {
		other, err := c.svcCtx.Dao.MessagesDAO.SelectNewestIncoming(
			ctx,
			in.UserId,
			atMax.DialogId1,
			atMax.DialogId2,
			in.UserId,
			maxId)
		if err != nil {
			c.Logger.Errorf("msg.readHistoryV2 - incoming message: %v", err)
			return
		}
		newestOther = other
	}

	target := chooseOutboxReadMessage(in.UserId, atMax, newestOther)
	if target == nil {
		return
	}

	switch in.PeerType {
	case mtproto.PEER_USER:
		_, _ = c.svcCtx.Dao.InboxClient.InboxReadOutboxHistory(ctx, &inbox.TLInboxReadOutboxHistory{
			UserId:             target.SenderUserId,
			PeerType:           in.PeerType,
			PeerId:             in.UserId,
			MaxDialogMessageId: target.DialogMessageId,
		})
	case mtproto.PEER_CHAT:
		_, _ = c.svcCtx.Dao.InboxClient.InboxReadOutboxHistory(ctx, &inbox.TLInboxReadOutboxHistory{
			UserId:             target.SenderUserId,
			PeerType:           in.PeerType,
			PeerId:             in.PeerId,
			MaxDialogMessageId: target.DialogMessageId,
		})
	}
}
