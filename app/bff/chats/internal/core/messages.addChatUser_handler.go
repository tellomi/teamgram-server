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

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *ChatsCore) addChatUser(chatId int64, userId *mtproto.InputUser, fwdLimit int32) (*mtproto.Updates, *mtproto.MissingInvitee, error) {
	var (
		err       error
		addUser   = mtproto.FromInputUser(c.MD.UserId, userId)
		chat      *mtproto.MutableChat
		inviterId int64
		isBot     = false
	)

	if !addUser.IsUser() || addUser.PeerId == 0 {
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.addChatUser - error: %v", err)
		return nil, nil, err
	}

	if !c.MD.IsAdmin {
		inviterId = c.MD.UserId

		// 400	USERS_TOO_MUCH	The maximum number of users has been exceeded (to create a chat, for example)
		// 400	USER_ALREADY_PARTICIPANT	The user is already in the group
		// 400	USER_ID_INVALID	The provided user ID is invalid
		// 403	USER_NOT_MUTUAL_CONTACT	The provided user is not a mutual contact
		// 403	USER_PRIVACY_RESTRICTED	The user's privacy settings do not allow you to do this
		users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{inviterId, addUser.PeerId},
		})

		me, _ := users.GetImmutableUser(inviterId)
		added, _ := users.GetImmutableUser(addUser.PeerId)
		if me == nil || added == nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("messages.addChatUser - error: %v", err)
			return nil, nil, err
		}

		if added.IsBot() && added.BotNochats() {
			err = mtproto.ErrBotGroupsBlocked
			c.Logger.Errorf("messages.addChatUser - error: %v", err)
			return nil, nil, err
		}

		if missing := c.chatInviteRestricted(inviterId, addUser.PeerId); missing != nil {
			return nil, missing, nil
		}

		isBot = added.IsBot()
	} else {
		inviterId = 0
	}

	chat, err = c.svcCtx.Dao.ChatClient.Client().ChatAddChatUser(c.ctx, &chatpb.TLChatAddChatUser{
		ChatId:    chatId,
		InviterId: inviterId,
		UserId:    addUser.PeerId,
		IsBot:     isBot,
	})
	//request.ChatId, md.UserId, peer.PeerId)
	if err != nil {
		c.Logger.Errorf("addChatUser error: %v", err)
		return nil, nil, err
	}

	fromId := c.MD.UserId
	if c.MD.IsAdmin {
		fromId = chat.Creator()
	}

	rUpdates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(
		c.ctx,
		&msgpb.TLMsgSendMessageV2{
			UserId:    fromId,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  mtproto.PEER_CHAT,
			PeerId:    chatId,
			Message: []*msgpb.OutboxMessage{
				msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
					NoWebpage:    true,
					Background:   false,
					RandomId:     rand.Int63(),
					Message:      chat.MakeMessageService(fromId, mtproto.MakeMessageActionChatAddUser(addUser.PeerId)),
					ScheduleDate: nil,
				}).To_OutboxMessage(),
			},
		})

	if err != nil {
		c.Logger.Errorf("addChatUser error: %v", err)
		return nil, nil, err
	}

	return rUpdates, nil, nil
}

// chatInviteRestricted applies the invitee's chat-invite privacy.
// A nil result means the invite is allowed. The new addChatUser result
// carries the missing invitee; the legacy method turns it into
// USER_PRIVACY_RESTRICTED.
func (c *ChatsCore) chatInviteRestricted(inviter, invitee int64) *mtproto.MissingInvitee {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || invitee == 0 || inviter == invitee {
		return nil
	}
	if userclient.EitherBlocked(c.ctx, c.svcCtx.Dao.UserClient, inviter, invitee) {
		return mtproto.MakeTLMissingInvitee(&mtproto.MissingInvitee{UserId: invitee}).To_MissingInvitee()
	}
	ok, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
		UserId:  invitee,
		KeyType: mtproto.CHAT_INVITE,
		PeerId:  inviter,
	})
	if err != nil || mtproto.FromBool(ok) {
		return nil
	}
	return mtproto.MakeTLMissingInvitee(&mtproto.MissingInvitee{UserId: invitee}).To_MissingInvitee()
}
