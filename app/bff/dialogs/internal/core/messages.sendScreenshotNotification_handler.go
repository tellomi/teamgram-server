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
	"math/rand"
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// MessagesSendScreenshotNotification
// messages.sendScreenshotNotification#c97df020 peer:InputPeer reply_to:InputReplyTo random_id:long = Updates;
//
// https://core.telegram.org/method/messages.sendScreenshotNotification
// https://core.telegram.org/constructor/messageActionScreenshotTaken
//
// The client calls this after a screenshot of a private chat. The result is
// a service message in that chat. Groups and channels are not private chats,
// so they are PEER_ID_INVALID. reply_to (and the legacy reply_to_msg_id)
// attaches the notice to a message.
func (c *DialogsCore) MessagesSendScreenshotNotification(in *mtproto.TLMessagesSendScreenshotNotification) (*mtproto.Updates, error) {
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil || !peer.IsUser() || peer.PeerId == 0 {
		c.Logger.Errorf("messages.sendScreenshotNotification - error: peer %v", peer)
		return nil, mtproto.ErrPeerIdInvalid
	}
	randomID := in.GetRandomId()
	if randomID == 0 {
		randomID = rand.Int63()
	}
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:     true,
		FromId:  mtproto.MakePeerUser(c.MD.UserId),
		PeerId:  peer.ToPeer(),
		Date:    int32(time.Now().Unix()),
		Action:  mtproto.MakeMessageActionScreenshotTaken(),
		ReplyTo: screenshotReply(in),
	}).To_Message()
	if peer.IsSelfUser(c.MD.UserId) {
		message.SavedPeerId = peer.ToPeer()
	}
	updates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_USER,
		PeerId:    peer.PeerId,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				NoWebpage: true,
				RandomId:  randomID,
				Message:   message,
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", err)
		return nil, err
	}
	if updates == nil {
		updates = mtproto.MakeEmptyUpdates()
	}
	return updates, nil
}

func screenshotReply(in *mtproto.TLMessagesSendScreenshotNotification) *mtproto.MessageReplyHeader {
	if in == nil {
		return nil
	}
	var id int32
	if in.GetReplyTo() != nil {
		id = in.GetReplyTo().GetReplyToMsgId()
	}
	if id == 0 {
		id = in.GetReplyToMsgId()
	}
	if id == 0 {
		return nil
	}
	return mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:           id,
		ReplyToMsgId_INT32:     id,
		ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(id),
	}).To_MessageReplyHeader()
}
