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
	"github.com/teamgram/teamgram-server/pkg/phonenumber"

	"google.golang.org/grpc/status"
)

// AuthCancelCode
// auth.cancelCode#1f040578 phone_number:string phone_code_hash:string = Bool;
func (c *AuthorizationCore) AuthCancelCode(in *mtproto.TLAuthCancelCode) (*mtproto.Bool, error) {
	// 客户端发送的手机号格式为: "+86 111 1111 1111"，归一化
	phone, err := phonenumber.CheckAndGetPhoneNumber(in.GetPhoneNumber())
	if err != nil {
		c.Logger.Errorf("check phone_number error - %v", err)
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if in.GetPhoneCodeHash() == "" {
		return nil, mtproto.ErrPhoneCodeHashEmpty
	}
	if c.MD == nil || c.MD.PermAuthKeyId == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}

	// A wrong hash must not delete a different pending code.
	if _, err = c.svcCtx.Dao.GetPhoneCode(c.ctx, c.MD.PermAuthKeyId, phone, in.GetPhoneCodeHash()); err != nil {
		if status.Convert(err).Message() == "PHONE_CODE_INVALID" {
			c.Logger.Errorf("auth.cancelCode - hash: %v", err)
			return nil, err
		}
		// Already gone. The login screen can leave.
		return mtproto.BoolTrue, nil
	}
	if c.svcCtx.AuthLogic == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = c.svcCtx.AuthLogic.DoAuthCancelCode(c.ctx, c.MD.PermAuthKeyId, phone, in.GetPhoneCodeHash()); err != nil {
		c.Logger.Errorf("auth.cancelCode - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
