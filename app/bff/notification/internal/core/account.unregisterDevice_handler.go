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
)

// AccountUnregisterDevice
// account.unregisterDevice#6a0d3206 token_type:int token:string other_uids:Vector<long> = Bool;
func (c *NotificationCore) AccountUnregisterDevice(in *mtproto.TLAccountUnregisterDevice) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	token := strings.TrimSpace(in.GetToken())
	if token == "" || in.GetTokenType() <= 0 {
		return nil, mtproto.ErrTokenInvalid
	}
	if c.svcCtx.Devices == nil {
		c.Logger.Errorf("account.unregisterDevice: device store is not configured")
		return nil, mtproto.ErrInternalServerError
	}
	if err := c.svcCtx.Devices.Delete(c.ctx, c.MD.UserId, in.GetTokenType(), token, in.GetOtherUids()); err != nil {
		c.Logger.Errorf("account.unregisterDevice - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
