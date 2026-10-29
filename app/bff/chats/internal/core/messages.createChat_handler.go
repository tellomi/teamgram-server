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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (c *ChatsCore) createChat(iUsers []*mtproto.InputUser, chatTitle string, ttlPeriod *wrapperspb.Int32Value) (*mtproto.Messages_InvitedUsers, error) {
	var chatUserIdList []int64

	// check chat title
	if chatTitle == "" {
		err := mtproto.ErrChatTitleEmpty
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	// Official Telegram iOS (layer 232) calls messages.createChat with an empty
	// users vector to create a group that contains only the creator. That client
	// maps USERS_TOO_FEW to "you can't create a group with these users due to
	// their privacy settings", so an empty list must succeed.
	if len(iUsers) > 200-1 {
		err := mtproto.ErrUsersTooMuch
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	seen := map[int64]struct{}{c.MD.UserId: {}}
	chatUserIdList = []int64{c.MD.UserId}
	for _, u := range iUsers {
		if u == nil || u.PredicateName != mtproto.Predicate_inputUser {
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("messages.createChat - error: %v", err)
			return nil, err
		}
		if _, ok := seen[u.UserId]; ok {
			continue
		}
		seen[u.UserId] = struct{}{}
		chatUserIdList = append(chatUserIdList, u.UserId)
	}

	users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: chatUserIdList,
	})
	if users == nil {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	me, ok := users.GetImmutableUser(c.MD.UserId)
	if !ok || me == nil {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}
	if me.Restricted() {
		err := mtproto.ErrUserRestricted
		c.Logger.Errorf("messages.createChat - error: %v", err)
		return nil, err
	}

	specs := make([]chatMemberSpec, 0, len(chatUserIdList)-1)
	for _, id := range chatUserIdList[1:] {
		addUser, found := users.GetImmutableUser(id)
		if !found || addUser == nil {
			err := mtproto.ErrInputUserDeactivated
			c.Logger.Errorf("messages.createChat - error: %v", err)
			return nil, err
		}
		spec := chatMemberSpec{
			UserID:     addUser.Id(),
			IsBot:      addUser.IsBot(),
			BotNoChats: addUser.BotNochats(),
		}
		if !spec.IsBot {
			rules, _ := c.svcCtx.Dao.UserClient.UserGetPrivacy(c.ctx, &userpb.TLUserGetPrivacy{
				UserId:  addUser.Id(),
				KeyType: mtproto.CHAT_INVITE,
			})
			if rules != nil {
				spec.Rules = rules.Datas
			}
		} else if !spec.BotNoChats {
			c.Logger.Errorf("user is bot and nochats, ignore %d", id)
		}
		specs = append(specs, spec)
	}

	userAddList, botAddList, missingInvitees := planChatInvites(
		c.MD.UserId,
		specs,
		func(id, checkId int64) bool {
			contact, _ := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
				UserId: id,
				Id:     checkId,
			})
			return mtproto.FromBool(contact)
		},
		func(checkId int64, idList []int64) bool {
			chatIdList, _ := mtproto.SplitChatAndChannelIdList(idList)
			return c.svcCtx.Dao.ChatClient.CheckParticipantIsExist(c.ctx, checkId, chatIdList)
		},
	)
	for _, miss := range missingInvitees {
		c.Logger.Errorf("chatInvite privacy, not adding %d", miss.GetUserId())
	}

	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatCreateChat2(c.ctx, &chatpb.TLChatCreateChat2{
		CreatorId:  c.MD.UserId,
		UserIdList: userAddList,
		Title:      chatTitle,
		Bots:       botAddList,
		TtlPeriod:  ttlPeriod,
	})
	if err != nil {
		c.Logger.Errorf("createChat duplicate: %v", err)
		return nil, err
	}

	outMessages := []*msgpb.OutboxMessage{
		msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage:  true,
			Background: false,
			RandomId:   rand.Int63(),
			Message: mtproto.MakeTLMessageService(&mtproto.Message{
				Out:                  true,
				Mentioned:            false,
				MediaUnread:          false,
				ReactionsArePossible: false,
				Silent:               false,
				Post:                 false,
				Legacy:               false,
				Id:                   0,
				FromId:               mtproto.MakePeerUser(c.MD.UserId),
				PeerId:               mtproto.MakePeerChat(chat.Id()),
				SavedPeerId:          nil,
				ReplyTo:              nil,
				Date:                 int32(time.Now().Unix()),
				Action: mtproto.MakeTLMessageActionChatCreate(&mtproto.MessageAction{
					Title:            chatTitle,
					Title_FLAGSTRING: mtproto.MakeFlagsString(chatTitle),
					Title_STRING:     chatTitle,
					Users:            append(userAddList, botAddList...),
				}).To_MessageAction(),
				Reactions: nil,
				TtlPeriod: ttlPeriod,
			}).To_Message(),
			ScheduleDate: nil,
		}).To_OutboxMessage(),
	}
	if ttlPeriod != nil {
		outMessages = append(outMessages, msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage:  true,
			Background: false,
			RandomId:   rand.Int63(),
			Message: mtproto.MakeTLMessageService(&mtproto.Message{
				Out:                  true,
				Mentioned:            false,
				MediaUnread:          false,
				ReactionsArePossible: true,
				Silent:               false,
				Post:                 false,
				Legacy:               false,
				Id:                   0,
				FromId:               mtproto.MakePeerUser(c.MD.UserId),
				PeerId:               mtproto.MakePeerChat(chat.Id()),
				SavedPeerId:          nil,
				ReplyTo:              nil,
				Date:                 int32(time.Now().Unix()),
				Action: mtproto.MakeTLMessageActionSetMessagesTTL(&mtproto.MessageAction{
					Period:          ttlPeriod.Value,
					AutoSettingFrom: mtproto.MakeFlagsInt64(c.MD.UserId),
				}).To_MessageAction(),
				Reactions: nil,
				TtlPeriod: nil,
			}).To_Message(),
			ScheduleDate: nil,
		}).To_OutboxMessage())
	}
	// TODO: add attach_data (chat and chat_participants)
	rValue, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_CHAT,
		PeerId:    chat.Chat.Id,
		Message:   outMessages,
	})
	if err != nil {
		// The chat row is already committed. CHAT_WRITE_FORBIDDEN here means
		// the participant list could not be loaded (production: missing
		// chat_participants.rank2), not that a privacy rule blocked the create.
		// Returning that 403 makes Telegram iOS show the privacy alert.
		if sendFailureKeepsChat(err) {
			c.Logger.Errorf("messages.createChat - chat %d was created but the service message was rejected (%v); returning the chat", chat.Id(), err)
			msgs := make([]*mtproto.Message, 0, len(outMessages))
			for _, box := range outMessages {
				if box != nil && box.Message != nil {
					msgs = append(msgs, box.Message)
				}
			}
			rValue = updatesForUnsentCreate(c.MD.UserId, chat, msgs)
		} else {
			c.Logger.Errorf("messages.createChat - error: %v", err)
			return nil, err
		}
	}

	return mtproto.MakeTLMessagesInvitedUsers(&mtproto.Messages_InvitedUsers{
		Updates:         rValue,
		MissingInvitees: missingInvitees,
	}).To_Messages_InvitedUsers(), nil
}
