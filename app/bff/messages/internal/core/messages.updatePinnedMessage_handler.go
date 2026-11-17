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
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesUpdatePinnedMessage
// messages.updatePinnedMessage#d2aaf7ec flags:# silent:flags.0?true unpin:flags.1?true pm_oneside:flags.2?true peer:InputPeer id:int = Updates;
func (c *MessagesCore) MessagesUpdatePinnedMessage(in *mtproto.TLMessagesUpdatePinnedMessage) (*mtproto.Updates, error) {
	var (
		peer     = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
		rUpdates *mtproto.Updates
	)

	if peer.IsChannel() {
		ups, err := channel.Shared().PinMessage(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, peer.PeerId, in.Id, in.Unpin)
		if err != nil {
			return nil, err
		}
		c.rememberPin(peer, in.Id, in.Unpin)
		return ups, nil
	}
	if peer.IsChat() {
		if err := c.basicPinAllowed(peer.PeerId); err != nil {
			return nil, err
		}
	}
	if !peer.IsChatOrUser() {
		c.Logger.Errorf("invalid peer: %v", in.Peer)
		err := mtproto.ErrPeerIdInvalid
		return nil, err
	}

	rUpdates, err := c.svcCtx.Dao.MsgClient.MsgUpdatePinnedMessage(c.ctx, &msgpb.TLMsgUpdatePinnedMessage{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		Silent:    in.Silent,
		Unpin:     in.Unpin,
		PmOneside: in.PmOneside,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Id:        in.Id,
	})
	if err != nil {
		c.Logger.Errorf("messages.updatePinnedMessage - error: %v", in.Peer)
		return nil, err
	}

	return rUpdates, nil
}

// rememberPin copies the channel pin onto dialogs.pinned_msg_id so
// channels.getFullChannel can fill pinned_msg_id even when the channel
// feature row does not have it yet.
func (c *MessagesCore) rememberPin(peer *mtproto.PeerUtil, id int32, unpin bool) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || peer == nil {
		return
	}
	pinned := id
	if unpin {
		pinned = 0
	}
	// Update the existing dialog only. InsertOrUpdate would set deleted=0 and
	// put a channel the pinner has already removed back into the chat list.
	_, _ = c.svcCtx.Dao.DialogClient.DialogUpdateUserPinnedMessage(c.ctx, &dialog.TLDialogUpdateUserPinnedMessage{
		UserId:      c.MD.UserId,
		PeerType:    peer.PeerType,
		PeerId:      peer.PeerId,
		PinnedMsgId: pinned,
	})
}

// basicPinAllowed lets a non-admin pin when default_banned_rights does not
// ban pin_messages. A ban returns CHAT_ADMIN_REQUIRED.
func (c *MessagesCore) basicPinAllowed(chatID int64) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return nil
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: chatID})
	if err != nil {
		return err
	}
	p, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	admin := ok && (p.IsChatMemberCreator() || p.IsChatMemberAdmin())
	return mtproto.MemberPinForbidden(int32(time.Now().Unix()), admin, []*mtproto.ChatBannedRights{chat.DefaultBannedRights()})
}
