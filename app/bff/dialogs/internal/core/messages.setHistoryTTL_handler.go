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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesSetHistoryTTL
// messages.setHistoryTTL#b80e5fe4 peer:InputPeer period:int = Updates;
//
// https://core.telegram.org/method/messages.setHistoryTTL
// https://core.telegram.org/constructor/messageActionSetMessagesTTL
//
// period is the new auto-delete timer in seconds. 0 disables it.
// Values outside 0..366 days are TTL_PERIOD_INVALID, same as
// messages.setDefaultHistoryTTL.
//
// The dialog (or channel) ttl_period is still stored, and the reply still
// carries updatePeerHistoryTTL. A messageActionSetMessagesTTL service message
// is also written into the peer history so the conversation shows the notice.
// period 0 is sent as a real action: the constructor's period field is a
// required int, and clients render the disabled state from 0. auto_setting_from
// stays unset because this method is a manual change.
func (c *DialogsCore) MessagesSetHistoryTTL(in *mtproto.TLMessagesSetHistoryTTL) (*mtproto.Updates, error) {
	period := in.GetPeriod()
	if period < 0 || period > 366*86400 {
		return nil, mtproto.ErrTtlPeriodInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		return channel.Shared().SetHistoryTTL(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, peer.PeerId, period)
	}
	if _, err := c.svcCtx.Dao.DialogClient.DialogSetHistoryTTL(c.ctx, &dialog.TLDialogSetHistoryTTL{
		UserId:    c.MD.UserId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		TtlPeriod: period,
	}); err != nil {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", err)
		return nil, err
	}

	sendType, sendID := peer.PeerType, peer.PeerId
	if peer.PeerType == mtproto.PEER_SELF {
		// msg.sendMessageV2 accepts user, chat, and channel. Saved messages
		// are a user dialog with the owner.
		sendType = mtproto.PEER_USER
		if sendID == 0 {
			sendID = c.MD.UserId
		}
	}
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:         true,
		FromId:      mtproto.MakePeerUser(c.MD.UserId),
		PeerId:      peer.ToPeer(),
		Date:        int32(time.Now().Unix()),
		Action:      mtproto.MakeMessageActionSetMessagesTTL(period),
		SavedPeerId: nil,
	}).To_Message()
	if peer.IsSelfUser(c.MD.UserId) {
		message.SavedPeerId = peer.ToPeer()
	}
	rUpdates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  sendType,
		PeerId:    sendID,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				NoWebpage:    true,
				Background:   false,
				RandomId:     rand.Int63(),
				Message:      message,
				ScheduleDate: nil,
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", err)
		return nil, err
	}

	ttlUpdate := historyTTLUpdate(peer.ToPeer(), period)
	if rUpdates == nil {
		rUpdates = mtproto.MakeUpdatesByUpdates(ttlUpdate)
	} else {
		rUpdates.PushBackUpdate(ttlUpdate)
	}
	c.syncHistoryTTL(peer, period)
	return rUpdates, nil
}

// historyTTLUpdate is updatePeerHistoryTTL. A positive period sets the flag.
// Zero omits it, which is how clients learn the timer was turned off.
func historyTTLUpdate(peer *mtproto.Peer, period int32) *mtproto.Update {
	var ttl *wrapperspb.Int32Value
	if period > 0 {
		ttl = &wrapperspb.Int32Value{Value: period}
	}
	return mtproto.MakeTLUpdatePeerHistoryTTL(&mtproto.Update{
		Peer_PEER: peer,
		TtlPeriod: ttl,
	}).To_Update()
}

// syncHistoryTTL delivers updatePeerHistoryTTL to the caller's other sessions
// and to the other participants. The service message itself is already fanned
// out by msg.sendMessageV2, which allocates pts and writes both histories.
// In a private chat the peer on the other side is the caller, not the target
// the caller named.
func (c *DialogsCore) syncHistoryTTL(peer *mtproto.PeerUtil, period int32) {
	selfUpdate := mtproto.MakeUpdatesByUpdates(historyTTLUpdate(peer.ToPeer(), period))
	if _, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       selfUpdate,
	}); err != nil {
		c.Logger.Errorf("messages.setHistoryTTL - sync not me: %v", err)
	}

	switch peer.PeerType {
	case mtproto.PEER_USER:
		if peer.PeerId == 0 || peer.PeerId == c.MD.UserId {
			return
		}
		if _, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
			UserId:  peer.PeerId,
			Updates: mtproto.MakeUpdatesByUpdates(historyTTLUpdate(mtproto.MakePeerUser(c.MD.UserId), period)),
		}); err != nil {
			c.Logger.Errorf("messages.setHistoryTTL - sync peer: %v", err)
		}
	case mtproto.PEER_CHAT:
		chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil || chat == nil {
			c.Logger.Errorf("messages.setHistoryTTL - chat participants: %v", err)
			return
		}
		chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
			if userId == c.MD.UserId || participant == nil || !participant.IsChatMemberStateNormal() {
				return nil
			}
			if _, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
				UserId:  userId,
				Updates: selfUpdate,
			}); err != nil {
				c.Logger.Errorf("messages.setHistoryTTL - sync chat: %v", err)
			}
			return nil
		})
	}
}
