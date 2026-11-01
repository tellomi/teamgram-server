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
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetOnlines
// messages.getOnlines#6e2be050 peer:InputPeer = ChatOnlines;
//
// https://core.telegram.org/method/messages.getOnlines
//
// Returns how many members of a basic group or channel are online right now.
// account.updateStatus stores last_seen_at and an expires window (300s while
// the app is in the foreground, 0 once it goes offline). A member counts
// while that window is still open. Private chats are PEER_ID_INVALID.
func (c *DialogsCore) MessagesGetOnlines(in *mtproto.TLMessagesGetOnlines) (*mtproto.ChatOnlines, error) {
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	var ids []int64
	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		pIDList, err := c.svcCtx.Dao.ChatClient.ChatGetChatParticipantIdList(c.ctx, &chatpb.TLChatGetChatParticipantIdList{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getOnlines - error: %v", err)
			return nil, err
		}
		ids = pIDList.GetDatas()
	case mtproto.PEER_CHANNEL:
		var err error
		ids, err = channel.Shared().VisibleMemberIDs(c.ctx, c.MD.UserId, peer.PeerId)
		if err != nil {
			c.Logger.Errorf("messages.getOnlines - error: %v", err)
			return nil, err
		}
	default:
		c.Logger.Errorf("messages.getOnlines - error: peer %v", peer)
		return nil, mtproto.ErrPeerIdInvalid
	}

	now := time.Now().Unix()
	var n int32
	if len(ids) > 0 {
		seen, err := c.svcCtx.Dao.UserGetLastSeens(c.ctx, &userpb.TLUserGetLastSeens{Id: ids})
		if err != nil {
			c.Logger.Errorf("messages.getOnlines - error: %v", err)
			return nil, err
		}
		n = countOnline(now, seen.GetDatas())
	}
	return mtproto.MakeTLChatOnlines(&mtproto.ChatOnlines{Onlines: n}).To_ChatOnlines(), nil
}

// countOnline applies the same window account.updateStatus writes:
// expires > 0 and now < last_seen_at+expires.
func countOnline(now int64, rows []*userpb.LastSeenData) int32 {
	var n int32
	for _, row := range rows {
		if row == nil || row.LastSeenAt <= 0 || row.Expires <= 0 {
			continue
		}
		if now < row.LastSeenAt+int64(row.Expires) {
			n++
		}
	}
	return n
}
