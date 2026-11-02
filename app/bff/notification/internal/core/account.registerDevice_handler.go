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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/push"
)

// AccountRegisterDevice
// account.registerDevice#ec86017a flags:# no_muted:flags.0?true token_type:int token:string app_sandbox:Bool secret:bytes other_uids:Vector<long> = Bool;
func (c *NotificationCore) AccountRegisterDevice(in *mtproto.TLAccountRegisterDevice) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	token := strings.TrimSpace(in.GetToken())
	if token == "" || in.GetTokenType() <= 0 {
		return nil, mtproto.ErrTokenInvalid
	}
	if c.svcCtx.Devices == nil {
		c.Logger.Errorf("account.registerDevice: device store is not configured")
		return nil, mtproto.ErrInternalServerError
	}
	authKeyID := c.MD.PermAuthKeyId
	if authKeyID == 0 {
		authKeyID = c.MD.AuthId
	}
	err := c.svcCtx.Devices.Upsert(c.ctx, push.Device{
		AuthKeyID:  authKeyID,
		UserID:     c.MD.UserId,
		TokenType:  in.GetTokenType(),
		Token:      token,
		NoMuted:    in.GetNoMuted(),
		AppSandbox: mtproto.FromBool(in.GetAppSandbox()),
		Secret:     append([]byte(nil), in.GetSecret()...),
		OtherUIDs:  append([]int64(nil), in.GetOtherUids()...),
		LangCode:   deviceLang(c.MD),
	})
	if err != nil {
		c.Logger.Errorf("account.registerDevice - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// deviceLang is the app language from the current session. initConnection
// writes the same value onto auths.lang_code; registerDevice snapshots it
// onto the device so a push can still be localized if that row is missing.
func deviceLang(md *metadata.RpcMetadata) string {
	if md == nil {
		return ""
	}
	if lang := strings.TrimSpace(md.GetLangCode()); lang != "" {
		return lang
	}
	return strings.TrimSpace(md.GetSystemLangCode())
}
