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

// MessagesSearchSentMedia
// messages.searchSentMedia#107e31a0 q:string filter:MessagesFilter limit:int = messages.Messages;
// https://core.telegram.org/method/messages.searchSentMedia
//
// Returns media the current user sent, newest first. q matches the message
// text. There is no pagination; limit is capped at 100.
func (c *MessagesCore) MessagesSearchSentMedia(in *mtproto.TLMessagesSearchSentMedia) (*mtproto.Messages_Messages, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	kind, err := searchbox.MediaFor(in.GetFilter())
	if err != nil {
		c.Logger.Errorf("messages.searchSentMedia - error: %v", err)
		return nil, err
	}
	rows, err := c.scanSent(kind)
	if err != nil {
		c.Logger.Errorf("messages.searchSentMedia - error: %v", err)
		return nil, err
	}
	picked := searchbox.SentMedia(rows, in.GetQ(), in.GetLimit())
	msgs := messagesOf(picked)
	users, chats := c.peersOf(msgs)
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Chats:    chats,
		Users:    users,
	}).To_Messages_Messages(), nil
}
