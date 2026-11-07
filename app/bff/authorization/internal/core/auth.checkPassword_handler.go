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
	"github.com/teamgram/teamgram-server/app/bff/account/deletion"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AuthCheckPassword
// auth.checkPassword#d18b4d16 password:InputCheckPasswordSRP = auth.Authorization;
// https://core.telegram.org/method/auth.checkPassword
func (c *AuthorizationCore) AuthCheckPassword(in *mtproto.TLAuthCheckPassword) (*mtproto.Auth_Authorization, error) {
	var proof *mtproto.InputCheckPasswordSRP
	if in != nil {
		proof = in.Password
	}
	userID := c.userID()
	if userID == 0 {
		id, err := password.PendingUser(c.ctx, c.authKey())
		if err != nil {
			c.Logger.Errorf("auth.checkPassword - pending: %v", err)
			return nil, err
		}
		userID = id
	}
	if userID == 0 {
		c.Logger.Errorf("auth.checkPassword - error: %v", mtproto.ErrPasswordMissing)
		return nil, mtproto.ErrPasswordMissing
	}
	if err := password.Verify(c.ctx, userID, c.authKey(), proof); err != nil {
		c.Logger.Errorf("auth.checkPassword - error: %v", err)
		return nil, err
	}
	if err := password.ClearLogin(c.ctx, c.authKey()); err != nil {
		c.Logger.Errorf("auth.checkPassword - clear login: %v", err)
		return nil, err
	}
	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: userID,
	})
	if err != nil {
		c.Logger.Errorf("auth.checkPassword - user: %v", err)
		return nil, err
	}
	if user == nil {
		c.Logger.Errorf("auth.checkPassword - user is nil")
		return nil, mtproto.ErrInternalServerError
	}
	_ = deletion.Cancel(c.ctx, userID)
	c.pushLoginSecurityNotice(userID)
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		User: user.ToSelfUser(),
	}).To_Auth_Authorization(), nil
}
