// Copyright 2025 Teamgram Authors
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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UsersSuggestBirthday
// users.suggestBirthday#fc533372 id:InputUser birthday:Birthday = Updates;
//
// https://core.telegram.org/method/users.suggestBirthday
// https://core.telegram.org/constructor/messageActionSuggestBirthday
//
// Suggests a birthday to a mutual contact by writing a service message into
// the private chat. The contact confirms it with their own
// account.updateBirthday. Day and month are required. February 29 is allowed
// without a year. A year, when set, has to fall between 1900 and this year.
func (c *UserChannelProfilesCore) UsersSuggestBirthday(in *mtproto.TLUsersSuggestBirthday) (*mtproto.Updates, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if !validBirthday(in.GetBirthday()) {
		c.Logger.Errorf("users.suggestBirthday - invalid birthday")
		return nil, mtproto.ErrBirthdayInvalid
	}
	target := mtproto.FromInputUser(c.MD.UserId, in.GetId())
	if target.PeerType != mtproto.PEER_USER || target.PeerId == 0 || target.PeerId == c.MD.UserId {
		c.Logger.Errorf("users.suggestBirthday - error: user %v", target)
		return nil, mtproto.ErrUserIdInvalid
	}
	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: target.PeerId,
	})
	if err != nil || user == nil || user.User == nil || user.IsBot() {
		c.Logger.Errorf("users.suggestBirthday - error: user %d: %v", target.PeerId, err)
		return nil, mtproto.ErrUserIdInvalid
	}
	mutual, err := c.mutualContact(target.PeerId)
	if err != nil {
		c.Logger.Errorf("users.suggestBirthday - error: %v", err)
		return nil, err
	}
	if !mutual {
		return nil, mtproto.ErrUserNotMutualContact
	}

	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(c.MD.UserId),
		PeerId: mtproto.MakePeerUser(target.PeerId),
		Date:   int32(time.Now().Unix()),
		Action: mtproto.MakeTLMessageActionSuggestBirthday(&mtproto.MessageAction{
			Birthday: in.GetBirthday(),
		}).To_MessageAction(),
	}).To_Message()
	updates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_USER,
		PeerId:    target.PeerId,
		Message: []*msgpb.OutboxMessage{
			msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
				NoWebpage: true,
				RandomId:  rand.Int63(),
				Message:   message,
			}).To_OutboxMessage(),
		},
	})
	if err != nil {
		c.Logger.Errorf("users.suggestBirthday - error: %v", err)
		return nil, err
	}
	if updates == nil {
		updates = mtproto.MakeEmptyUpdates()
	}
	return updates, nil
}

func (c *UserChannelProfilesCore) mutualContact(id int64) (bool, error) {
	forward, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
		UserId: c.MD.UserId,
		Id:     id,
	})
	if err != nil {
		return false, err
	}
	if !mtproto.FromBool(forward) {
		return false, nil
	}
	back, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
		UserId: id,
		Id:     c.MD.UserId,
	})
	if err != nil {
		return false, err
	}
	return mtproto.FromBool(back), nil
}

func validBirthday(b *mtproto.Birthday) bool {
	if b == nil || b.Month < 1 || b.Month > 12 || b.Day < 1 {
		return false
	}
	days := [...]int32{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if b.Day > days[b.Month] {
		return false
	}
	if b.Year != nil {
		y := b.Year.GetValue()
		if y < 1900 || y > int32(time.Now().Year()) {
			return false
		}
	}
	return true
}
