// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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

package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/richmsg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesTranslateRichMessage
// messages.translateRichMessage#1a542004 flags:# peer:flags.0?InputPeer id:flags.0?Vector<int> text:flags.1?Vector<InputRichMessage> to_lang:string tone:flags.2?string = messages.TranslatedRichMessage;
// https://core.telegram.org/method/messages.translateText
func (c *MessagesCore) MessagesTranslateRichMessage(in *mtproto.TLMessagesTranslateRichMessage) (*mtproto.Messages_TranslatedRichMessage, error) {
	var self int64
	if c.MD != nil {
		self = c.MD.GetUserId()
	}
	return richmsg.Translate(c.ctx, self, in, c.loadPlain)
}

func (c *MessagesCore) loadPlain(ctx context.Context, self int64, peer *mtproto.PeerUtil, id int32) (string, []*mtproto.MessageEntity, bool) {
	if peer != nil && peer.IsChannel() {
		svc := channel.Shared()
		if svc != nil {
			box, err := svc.MessageBox(ctx, self, peer.PeerId, id)
			if err == nil && box != nil && box.GetMessage() != nil && box.GetMessage().GetPredicateName() != mtproto.Predicate_messageEmpty {
				msg := box.GetMessage()
				return msg.GetMessage(), msg.GetEntities(), true
			}
		}
		return "", nil, false
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.MessageClient == nil {
		return "", nil, false
	}
	list, err := c.svcCtx.MessageClient.MessageGetUserMessageList(ctx, &message.TLMessageGetUserMessageList{
		UserId: self,
		IdList: []int32{id},
	})
	if err != nil || list == nil {
		return "", nil, false
	}
	for _, box := range list.GetDatas() {
		if box == nil || box.GetMessage() == nil || box.GetMessage().GetPredicateName() == mtproto.Predicate_messageEmpty {
			continue
		}
		msg := box.GetMessage()
		if msg.GetId() != id {
			continue
		}
		return msg.GetMessage(), msg.GetEntities(), true
	}
	return "", nil, false
}
