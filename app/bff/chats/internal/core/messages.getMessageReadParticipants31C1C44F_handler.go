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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesGetMessageReadParticipants31C1C44F
// messages.getMessageReadParticipants#31c1c44f peer:InputPeer msg_id:int = Vector<ReadParticipantDate>;
func (c *ChatsCore) MessagesGetMessageReadParticipants31C1C44F(in *mtproto.TLMessagesGetMessageReadParticipants31C1C44F) (*mtproto.Vector_ReadParticipantDate, error) {
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	empty := &mtproto.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{}}

	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		pIdList, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatParticipantIdList(c.ctx, &chatpb.TLChatGetChatParticipantIdList{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		}
		if !participantListHas(pIdList.GetDatas(), c.MD.UserId) {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.PEER_CHANNEL:
	default:
		c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.MsgId <= 0 {
		return empty, nil
	}

	ownerID, msgID, err := c.outboxReadOwner(peer.PeerType, in.MsgId)
	if err != nil {
		c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
		return nil, err
	}
	if ownerID == 0 || msgID <= 0 {
		return empty, nil
	}

	// One indexed lookup. A failure returns an empty list so this RPC cannot
	// block the session the way a stuck getDifference does, and it never
	// returns the epoch (clients in UTC+8 show that as 08:00).
	got, err := c.svcCtx.Dao.MessageClient.MessageGetOutboxReadDate(c.ctx, &message.TLMessageGetOutboxReadDate{
		UserId:   ownerID,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		MsgId:    msgID,
	})
	if err != nil {
		c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
		return empty, nil
	}
	out := make([]*mtproto.ReadParticipantDate, 0, len(got.GetDatas()))
	for _, d := range got.GetDatas() {
		if d.GetUserId() == 0 || d.GetUserId() == c.MD.UserId || d.GetDate() <= 0 {
			continue
		}
		out = append(out, d)
	}
	return &mtproto.Vector_ReadParticipantDate{Datas: out}, nil
}

// outboxReadOwner is the user (or channel) the receipt was stored under,
// and the message id in that user's box. Channel ids are already global.
func (c *ChatsCore) outboxReadOwner(peerType, msgID int32) (int64, int32, error) {
	if peerType == mtproto.PEER_CHANNEL {
		return c.MD.UserId, msgID, nil
	}
	msgBox, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
		UserId: c.MD.UserId,
		Id:     msgID,
	})
	if err != nil {
		return 0, 0, err
	}
	if msgBox.GetSenderUserId() == 0 || msgBox.GetSenderUserId() == c.MD.UserId || msgBox.GetDialogMessageId() == 0 {
		return c.MD.UserId, msgID, nil
	}
	boxes, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageListByDataIdUserIdList(c.ctx, &message.TLMessageGetUserMessageListByDataIdUserIdList{
		Id:         msgBox.GetDialogMessageId(),
		UserIdList: []int64{msgBox.GetSenderUserId()},
	})
	if err != nil {
		return 0, 0, err
	}
	var senderBox int32
	if boxes == nil {
		return msgBox.GetSenderUserId(), msgID, nil
	}
	boxes.Walk(func(idx int, v *mtproto.MessageBox) {
		if v.GetUserId() == msgBox.GetSenderUserId() {
			senderBox = v.GetMessageId()
		}
	})
	if senderBox <= 0 {
		return msgBox.GetSenderUserId(), msgID, nil
	}
	return msgBox.GetSenderUserId(), senderBox, nil
}

func participantListHas(ids []int64, userID int64) bool {
	for _, id := range ids {
		if id == userID {
			return true
		}
	}
	return false
}
