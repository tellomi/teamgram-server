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
	"github.com/teamgram/teamgram-server/app/bff/messages/searchbox"
)

// MessagesGetRecentLocations
// messages.getRecentLocations#702a40e0 peer:InputPeer limit:int hash:long = messages.Messages;
// https://core.telegram.org/method/messages.getRecentLocations
//
// Returns at most one live location (messageMediaGeoLive) per sender that has
// not expired. A matching hash returns messages.messagesNotModified.
func (c *MessagesCore) MessagesGetRecentLocations(in *mtproto.TLMessagesGetRecentLocations) (*mtproto.Messages_Messages, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	peer, err := c.peerOrInvalid(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", err)
		return nil, err
	}
	rows, err := c.scanHistory(peer, 5)
	if err != nil {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", err)
		return nil, err
	}
	live := searchbox.PickLive(rows, nowUnix(), in.GetLimit())
	ids := make([]int32, 0, len(live))
	for _, row := range live {
		ids = append(ids, row.ID)
	}
	if in.GetHash() != 0 && in.GetHash() == searchbox.HashIDs(ids) {
		return mtproto.MakeTLMessagesMessagesNotModified(&mtproto.Messages_Messages{
			Count: int32(len(live)),
		}).To_Messages_Messages(), nil
	}
	msgs := messagesOf(live)
	users, chats := c.peersOf(msgs)
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Chats:    chats,
		Users:    users,
	}).To_Messages_Messages(), nil
}
