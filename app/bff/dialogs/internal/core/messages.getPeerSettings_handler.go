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
	"github.com/teamgram/teamgram-server/app/bff/accountassets"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetPeerSettings
// messages.getPeerSettings#efd9a6a2 peer:InputPeer = messages.PeerSettings;
func (c *DialogsCore) MessagesGetPeerSettings(in *mtproto.TLMessagesGetPeerSettings) (*mtproto.Messages_PeerSettings, error) {
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)

	peerSettings, err := c.svcCtx.UserClient.UserGetPeerSettings(c.ctx, &userpb.TLUserGetPeerSettings{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})

	if err != nil {
		// A missing row is already empty settings from the user service.
		// A store or RPC error is not that row.
		c.Logger.Errorf("messages.getPeerSettings - error: %v", err)
		return nil, err
	}

	users := []*mtproto.User{}
	if peer.PeerType == mtproto.PEER_USER && peer.PeerId != 0 && peer.PeerId != c.MD.UserId {
		if bot := accountassets.ApplyBusinessBar(peer.PeerId, c.MD.UserId, peerSettings); bot != nil {
			users = append(users, bot)
		}
	}

	return mtproto.MakeTLMessagesPeerSettings(&mtproto.Messages_PeerSettings{
		Settings: peerSettings,
		Chats:    []*mtproto.Chat{},
		Users:    users,
	}).To_Messages_PeerSettings(), nil
}
