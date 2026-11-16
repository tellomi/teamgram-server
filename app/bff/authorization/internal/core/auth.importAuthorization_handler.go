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
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AuthImportAuthorization
// auth.importAuthorization#a57a7dad id:long bytes:bytes = auth.Authorization;
//
// https://core.telegram.org/method/auth.importAuthorization
// The bytes must be the secret from auth.exportAuthorization for the same user.
// A hit binds this auth key and returns that user. Anything else is
// 400 AUTH_BYTES_INVALID. The blob is consumed, so a second import fails.
func (c *AuthorizationCore) AuthImportAuthorization(in *mtproto.TLAuthImportAuthorization) (*mtproto.Auth_Authorization, error) {
	if in == nil || in.GetId() == 0 || len(in.GetBytes()) == 0 {
		return nil, mtproto.ErrAuthBytesInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}
	userID, err := c.svcCtx.Dao.TakeExportedAuth(c.ctx, in.GetBytes())
	if err != nil || userID == 0 || userID != in.GetId() {
		c.Logger.Errorf("auth.importAuthorization - bytes")
		return nil, mtproto.ErrAuthBytesInvalid
	}
	if c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: userID,
	})
	if err != nil || user == nil || user.Id() == 0 || user.Deleted() {
		c.Logger.Errorf("auth.importAuthorization - user: %v", err)
		return nil, mtproto.ErrAuthBytesInvalid
	}
	if c.MD != nil && c.MD.PermAuthKeyId != 0 && c.svcCtx.Dao.AuthsessionClient != nil {
		if _, err = c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
			AuthKeyId: c.MD.PermAuthKeyId,
			UserId:    user.Id(),
		}); err != nil {
			c.Logger.Errorf("auth.importAuthorization - bind: %v", err)
			return nil, err
		}
	}
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		User: user.ToSelfUser(),
	}).To_Auth_Authorization(), nil
}
