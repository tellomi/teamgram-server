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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/account/botconn"
	"github.com/teamgram/teamgram-server/app/bff/botadmin"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
)

// AccountConfirmBotConnection
// account.confirmBotConnection#67ed1f68 bot_id:InputUser = Bool;
// https://core.telegram.org/method/account.confirmBotConnection
func (c *AccountCore) AccountConfirmBotConnection(in *mtproto.TLAccountConfirmBotConnection) (*mtproto.Bool, error) {
	var self int64
	if c.MD != nil {
		self = c.MD.GetUserId()
	}
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	var bot *mtproto.InputUser
	if in != nil {
		bot = in.GetBotId()
	}
	id, hash, err := botconn.Target(self, bot)
	if err != nil {
		return nil, err
	}
	if local, ok := botadmin.LocalBot(id); ok {
		if hash != local {
			return nil, mtproto.ErrBotIdInvalid
		}
		return botconn.Confirm(c.ctx, self, id, c.botConnKV())
	}
	ok, err := c.userIsBot(id, hash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrBotIdInvalid
	}
	return botconn.Confirm(c.ctx, self, id, c.botConnKV())
}

func (c *AccountCore) botConnKV() botconn.KV {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil
	}
	cache := c.svcCtx.Dao.Cache()
	if cache == nil {
		return nil
	}
	return cache
}

func (c *AccountCore) userIsBot(id, hash int64) (bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.UserClient == nil {
		return false, nil
	}
	flag, err := c.svcCtx.UserClient.UserIsBot(c.ctx, &userpb.TLUserIsBot{Id: id})
	if err != nil {
		if status.Convert(err).Message() == "USER_ID_INVALID" {
			return false, nil
		}
		return false, err
	}
	if !mtproto.FromBool(flag) {
		return false, nil
	}
	user, err := c.svcCtx.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: id})
	if err != nil {
		if status.Convert(err).Message() == "USER_ID_INVALID" {
			return false, nil
		}
		return false, err
	}
	if user == nil {
		return false, nil
	}
	stored := user.AccessHash()
	if stored != 0 && hash != stored {
		return false, nil
	}
	return true, nil
}
