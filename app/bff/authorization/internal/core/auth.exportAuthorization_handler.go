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
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
)

// AuthExportAuthorization
// auth.exportAuthorization#e5bfffcd dc_id:int = auth.ExportedAuthorization;
//
// https://core.telegram.org/method/auth.exportAuthorization
// bytes is a one-shot secret, not the DC number. importAuthorization rejects
// anything that was not issued here.
func (c *AuthorizationCore) AuthExportAuthorization(in *mtproto.TLAuthExportAuthorization) (*mtproto.Auth_ExportedAuthorization, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetDcId() == 0 {
		return nil, mtproto.ErrDcIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}
	raw, err := dao.NewExportBytes()
	if err != nil {
		c.Logger.Errorf("auth.exportAuthorization - bytes: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	if err = c.svcCtx.Dao.PutExportedAuth(c.ctx, raw, c.MD.UserId); err != nil {
		c.Logger.Errorf("auth.exportAuthorization - store: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLAuthExportedAuthorization(&mtproto.Auth_ExportedAuthorization{
		Id:    c.MD.UserId,
		Bytes: raw,
	}).To_Auth_ExportedAuthorization(), nil
}
