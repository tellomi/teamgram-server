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
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountSetAuthorizationTTL
// account.setAuthorizationTTL#bf899aa0 authorization_ttl_days:int = Bool;
//
// The RPC only stores users.authorization_ttl_days. It does not delete
// auth_users rows, reset authorizations, or log the current session out.
// A later request expires a session only when that session's tracked
// activity is already outside the stored window. The column default stays
// 180 days for accounts that never call this method.
func (c *AuthorizationCore) AccountSetAuthorizationTTL(in *mtproto.TLAccountSetAuthorizationTTL) (*mtproto.Bool, error) {
	value := in.GetAuthorizationTtlDays()
	if !ValidAuthorizationTTLDays(value) {
		c.Logger.Errorf("account.setAuthorizationTTL - invalid days")
		return nil, mtproto.ErrTtlDaysInvalid
	}

	result, err := c.svcCtx.Dao.UserClient.UserSetAuthorizationTTL(
		c.ctx,
		&user.TLUserSetAuthorizationTTL{
			UserId: c.MD.UserId,
			Ttl:    value,
		})
	return ttlReply(result, err)
}

// ttlReply returns the stored Bool. A nil result is an error, not success.
func ttlReply(result *mtproto.Bool, err error) (*mtproto.Bool, error) {
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

// ValidAuthorizationTTLDays is the set official clients send for the
// session inactivity window. Anything else is TTL_DAYS_INVALID and is not
// stored. This is not account.setAccountTTL (the self-destruct timer).
//
// Android SessionsActivity sends 7, 90, 183, or 365.
// Desktop SelfDestructionBox sessions sends 7, 30, 90, 180, or 365.
// iOS account.setAuthorizationTTL forwards the day chosen on that screen;
// one week is 7, and six months is 183 on Android and 180 on Desktop.
// 182, 548, and 730 stay accepted because accounts already store them.
// The unset default remains 180 (users.authorization_ttl_days).
func ValidAuthorizationTTLDays(days int32) bool {
	switch days {
	case 7, 30, 90, 180, 182, 183, 365, 548, 730:
		return true
	default:
		return false
	}
}
