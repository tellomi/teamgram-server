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
	"github.com/teamgram/teamgram-server/app/bff/messages/sendas"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// MessagesSaveDefaultSendAs
// messages.saveDefaultSendAs#ccfddf96 peer:InputPeer send_as:InputPeer = Bool;
// https://core.telegram.org/method/messages.saveDefaultSendAs
//
// Stores the peer that should be used when sending into a group or channel.
// send_as must be the current user or a channel returned by channels.getSendAs.
// channels.getSendAs then lists that peer first.
func (c *MessagesCore) MessagesSaveDefaultSendAs(in *mtproto.TLMessagesSaveDefaultSendAs) (*mtproto.Bool, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	peer, err := c.peerOrInvalid(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.saveDefaultSendAs - error: %v", err)
		return nil, err
	}
	if !peer.IsChat() && !peer.IsChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		if chat := channel.Shared().VisibleChat(c.ctx, c.MD.UserId, peer.PeerId); chat != nil && chat.GetLeft() {
			return nil, mtproto.ErrChannelInvalid
		}
	}
	sendAs, err := c.peerOrInvalid(in.GetSendAs())
	if err != nil {
		return nil, mtproto.ErrSendAsPeerInvalid
	}
	choice := sendas.Choice{Type: sendAs.PeerType, ID: sendAs.PeerId}
	switch {
	case sendAs.IsUser() && (sendAs.IsSelf() || sendAs.PeerId == c.MD.UserId):
		choice = sendas.Choice{Type: mtproto.PEER_USER, ID: c.MD.UserId}
	case sendAs.IsChannel() && c.canSendAsChannel(sendAs.PeerId):
	default:
		return nil, mtproto.ErrSendAsPeerInvalid
	}
	if err := sendas.Shared().Set(c.ctx, c.MD.UserId, peer.PeerType, peer.PeerId, choice); err != nil {
		c.Logger.Errorf("messages.saveDefaultSendAs - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.BoolTrue, nil
}

func (c *MessagesCore) canSendAsChannel(channelID int64) bool {
	_, ids, err := channel.Shared().SendAs(c.ctx, c.MD.UserId)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == channelID {
			return true
		}
	}
	return false
}
