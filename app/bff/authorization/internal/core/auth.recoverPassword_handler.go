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
	"github.com/teamgram/teamgram-server/app/bff/codelogin"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
)

// AuthRecoverPassword
// auth.recoverPassword#37096c70 flags:# code:string new_settings:flags.0?account.PasswordInputSettings = auth.Authorization;
// https://core.telegram.org/method/auth.recoverPassword
func (c *AuthorizationCore) AuthRecoverPassword(in *mtproto.TLAuthRecoverPassword) (*mtproto.Auth_Authorization, error) {
	var (
		code     string
		settings *mtproto.Account_PasswordInputSettings
	)
	if in != nil {
		code = in.Code
		settings = in.NewSettings
	}
	userID, err := c.passwordUserID()
	if err != nil {
		c.Logger.Errorf("auth.recoverPassword - pending: %v", err)
		return nil, err
	}
	if err = c.codes().RecoverPassword(c.ctx, userID, code, settings); err != nil {
		c.Logger.Errorf("auth.recoverPassword - error: %v", err)
		return nil, err
	}
	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: userID,
	})
	if err != nil {
		c.Logger.Errorf("auth.recoverPassword - user: %v", err)
		return nil, err
	}
	if user == nil {
		c.Logger.Errorf("auth.recoverPassword - user is nil")
		return nil, mtproto.ErrInternalServerError
	}
	if stage := enforcement.LoginStage(user.Id(), "", "", true, time.Now()); !stage.BindOrdinary {
		if err = enforcement.RPC(stage.Code); err != nil {
			c.Logger.Errorf("auth.recoverPassword - sanction")
			return nil, err
		}
	}
	_, _ = c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: c.authKey(),
		UserId:    user.Id(),
	})
	c.pushLoginSecurityNotice(user.Id())
	return codelogin.Authorization(user.ToSelfUser()), nil
}
