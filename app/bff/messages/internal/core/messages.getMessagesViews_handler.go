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
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetMessagesViews
// messages.getMessagesViews#5784d3e1 peer:InputPeer id:Vector<int> increment:Bool = messages.MessageViews;
//
// https://core.telegram.org/method/messages.getMessagesViews
//
// A channel post returns its own view counter. increment counts the caller
// once. A private chat or basic group only has views when the message is a
// forward of a channel post; the counter is that original post's. Other
// messages return a MessageViews with the views flag unset. The result
// vector lines up with id.
func (c *MessagesCore) MessagesGetMessagesViews(in *mtproto.TLMessagesGetMessagesViews) (*mtproto.Messages_MessageViews, error) {
	if len(in.GetId()) == 0 {
		err := mtproto.ErrInputRequestInvalid
		c.Logger.Errorf("messages.getMessagesViews - error: %v", err)
		return nil, err
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	increment := mtproto.FromBool(in.GetIncrement())

	switch peer.PeerType {
	case mtproto.PEER_USER:
		mutableUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{c.MD.UserId, peer.PeerId},
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessagesViews - error: %v", err)
			return nil, err
		}
		if mutableUsers.Length() != 2 {
			c.Logger.Errorf("messages.getMessagesViews - error: peer not found (%v)", peer)
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.PEER_CHAT:
		mutableChat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessagesViews - error: chat %v: %v", peer.PeerId, err)
			return nil, mtproto.ErrChatIdInvalid
		}
		if _, ok := mutableChat.GetImmutableChatParticipant(c.MD.UserId); !ok {
			c.Logger.Errorf("messages.getMessagesViews - error: not in chat %v", peer.PeerId)
			return nil, mtproto.ErrChatIdInvalid
		}
	case mtproto.PEER_CHANNEL:
		views, err := channel.Shared().MessageViews(c.ctx, c.MD.UserId, peer.PeerId, in.Id, increment)
		if err != nil {
			c.Logger.Errorf("messages.getMessagesViews - error: %v", err)
			return nil, err
		}
		return packMessageViews(views), nil
	case mtproto.PEER_SELF:
	default:
		c.Logger.Errorf("messages.getMessagesViews - error: invalid peer %v", peer)
		return nil, mtproto.ErrInputRequestInvalid
	}

	boxMsgList, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: in.Id,
	})
	if err != nil {
		c.Logger.Errorf("messages.getMessagesViews - error: %v", err)
		return nil, err
	}
	boxes := map[int32]*mtproto.MessageBox{}
	for _, box := range boxMsgList.GetDatas() {
		if box != nil {
			boxes[box.MessageId] = box
		}
	}
	cache := map[[2]int64]*mtproto.MessageViews{}
	views := make([]*mtproto.MessageViews, 0, len(in.Id))
	for _, id := range in.Id {
		box := boxes[id]
		if box == nil || box.Message == nil {
			views = append(views, emptyMessageViews())
			continue
		}
		src, post, ok := channelPostOf(box.Message)
		if !ok {
			views = append(views, emptyMessageViews())
			continue
		}
		key := [2]int64{src, int64(post)}
		if cached, hit := cache[key]; hit {
			views = append(views, cached)
			continue
		}
		got, err := channel.Shared().MessageViews(c.ctx, c.MD.UserId, src, []int32{post}, increment)
		var one *mtproto.MessageViews
		if err != nil || len(got) == 0 {
			one = emptyMessageViews()
		} else {
			one = got[0]
		}
		cache[key] = one
		views = append(views, one)
	}
	return packMessageViews(views), nil
}

func packMessageViews(views []*mtproto.MessageViews) *mtproto.Messages_MessageViews {
	if views == nil {
		views = []*mtproto.MessageViews{}
	}
	return mtproto.MakeTLMessagesMessageViews(&mtproto.Messages_MessageViews{
		Views: views,
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Messages_MessageViews()
}

func emptyMessageViews() *mtproto.MessageViews {
	return mtproto.MakeTLMessageViews(&mtproto.MessageViews{}).To_MessageViews()
}

func channelPostOf(msg *mtproto.Message) (int64, int32, bool) {
	fwd := msg.GetFwdFrom()
	if fwd == nil {
		return 0, 0, false
	}
	if from := fwd.GetFromId(); from.GetPredicateName() == mtproto.Predicate_peerChannel && fwd.GetChannelPost().GetValue() != 0 {
		return from.GetChannelId(), fwd.GetChannelPost().GetValue(), true
	}
	if saved := fwd.GetSavedFromPeer(); saved.GetPredicateName() == mtproto.Predicate_peerChannel && fwd.GetSavedFromMsgId().GetValue() != 0 {
		return saved.GetChannelId(), fwd.GetSavedFromMsgId().GetValue(), true
	}
	return 0, 0, false
}
