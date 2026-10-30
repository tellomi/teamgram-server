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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/readoutbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/pkg/readtime"
)

// propagateOutboxReads tells the authors of messages inside this read that
// someone else has seen them. The reader is never one of those authors, so
// messages.readHistory from the sender's own client (the chat is open on iOS)
// does not move their read_outbox_max_id.
func (c *MsgCore) propagateOutboxReads(ctx context.Context, in *msg.TLMsgReadHistoryV2, prevInbox, maxID int32, maxMsg *dataobject.MessagesDO) {
	if in.PeerType != mtproto.PEER_USER && in.PeerType != mtproto.PEER_CHAT {
		return
	}
	targets := readoutbox.Targets(in.UserId, c.messagesJustRead(ctx, in, prevInbox, maxID, maxMsg))
	for _, target := range targets {
		peerID := in.PeerId
		if in.PeerType == mtproto.PEER_USER {
			// For a private chat the outbox peer is the person who read.
			peerID = in.UserId
		}
		// Private receipts are written in inbox.readOutboxHistory, where the
		// peer is the reader. Groups do not carry the reader on that call.
		if in.PeerType == mtproto.PEER_CHAT {
			c.rememberOutboxRead(ctx, in.UserId, target)
		}
		_, _ = c.svcCtx.Dao.InboxClient.InboxReadOutboxHistory(ctx, &inbox.TLInboxReadOutboxHistory{
			UserId:             target.SenderID,
			PeerType:           in.PeerType,
			PeerId:             peerID,
			MaxDialogMessageId: target.DialogMessageID,
		})
	}
}

// rememberOutboxRead writes the reader's checkpoint on the sender's outbox
// before the inbox fan-out. Groups have no reader id on that fan-out, so this
// is the copy getMessageReadParticipants reads. A lookup or insert failure
// does not stop the read.
func (c *MsgCore) rememberOutboxRead(ctx context.Context, reader int64, target readoutbox.Target) {
	row, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageDataId(ctx, target.SenderID, target.DialogMessageID)
	if err != nil {
		c.Logger.Errorf("readHistory outbox date - error: %v", err)
		return
	}
	if row == nil || row.UserMessageBoxId <= 0 || row.SenderUserId != target.SenderID {
		return
	}
	when := readtime.Stamp(time.Now().Unix())
	if reader == 0 || target.SenderID == 0 || reader == target.SenderID || when <= 0 {
		return
	}
	// Peer dialog id is filled by the caller via the message row's peer.
	peerDialog := mtproto.MakePeerDialogId(row.PeerType, row.PeerId)
	if row.PeerType == mtproto.PEER_USER {
		peerDialog = mtproto.MakePeerDialogId(mtproto.PEER_USER, reader)
	}
	_, _, err = c.svcCtx.Dao.MessageReadOutboxDAO.InsertOrUpdate(ctx, &dataobject.MessageReadOutboxDO{
		UserId:            target.SenderID,
		PeerDialogId:      peerDialog,
		ReadUserId:        reader,
		ReadOutboxMaxId:   row.UserMessageBoxId,
		ReadOutboxMaxDate: when,
	})
	if err != nil {
		c.Logger.Errorf("readHistory outbox date - error: %v", err)
	}
}

func (c *MsgCore) messagesJustRead(ctx context.Context, in *msg.TLMsgReadHistoryV2, prevInbox, maxID int32, maxMsg *dataobject.MessagesDO) []readoutbox.Message {
	out := make([]readoutbox.Message, 0, 8)
	add := func(sender, dialogID int64, boxID int32) {
		if boxID <= prevInbox || boxID > maxID {
			return
		}
		out = append(out, readoutbox.Message{SenderID: sender, DialogMessageID: dialogID})
	}
	if maxMsg != nil {
		add(maxMsg.SenderUserId, maxMsg.DialogMessageId, maxMsg.UserMessageBoxId)
	}
	did := mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
	older, err := c.svcCtx.Dao.MessagesDAO.SelectBackwardByOffsetIdLimit(ctx, in.UserId, did.A, did.B, maxID, 200)
	if err != nil {
		c.Logger.Errorf("readHistory outbox window - error: %v", err)
		return out
	}
	for i := range older {
		add(older[i].SenderUserId, older[i].DialogMessageId, older[i].UserMessageBoxId)
	}
	return out
}
