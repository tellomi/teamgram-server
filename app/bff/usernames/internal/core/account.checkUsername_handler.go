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

	"google.golang.org/grpc/status"
)

// AccountCheckUsername
// account.checkUsername#2714d86c username:string = Bool;
//
// A free name, including the caller's current name, returns boolTrue.
// Occupied names return USERNAME_OCCUPIED. Names outside
// 3–32 [A-Za-z0-9_], or that do not start with a letter, return
// USERNAME_INVALID. Other failures are logged and returned as
// USERNAME_INVALID so the client does not retry them as an internal error.
func (c *UsernamesCore) AccountCheckUsername(in *mtproto.TLAccountCheckUsername) (*mtproto.Bool, error) {
	if c.MD == nil {
		c.Logger.Errorf("account.checkUsername#2714d86c - missing metadata")
		return nil, mtproto.ErrUsernameInvalid
	}
	if in == nil || !user.CheckUsernameInvalid(in.GetUsername()) {
		c.Logger.Errorf("account.checkUsername#2714d86c - format error: %v", mtproto.ErrUsernameInvalid)
		return nil, mtproto.ErrUsernameInvalid
	}

	existed, err := c.svcCtx.Dao.UserClient.UserCheckAccountUsername(c.ctx, &user.TLUserCheckAccountUsername{
		UserId:   c.MD.UserId,
		Username: in.GetUsername(),
	})
	if err != nil {
		c.Logger.Errorf("account.checkUsername#2714d86c - error: %v", err)
		return nil, usernameRPCError(err)
	}

	switch existed.GetPredicateName() {
	case user.Predicate_usernameExisted, user.Predicate_usernameExistedNotMe:
		c.Logger.Errorf("account.checkUsername#2714d86c - exists username: %v", mtproto.ErrUsernameOccupied)
		return nil, mtproto.ErrUsernameOccupied
	default:
		return mtproto.BoolTrue, nil
	}
}

// usernameRPCError passes client errors through (USERNAME_INVALID,
// USERNAME_OCCUPIED, CHAT_ADMIN_REQUIRED, and the other 4xx codes).
// Internal and unknown failures become USERNAME_INVALID. iOS retries 500s
// and otherwise stays on "Checking username...".
func usernameRPCError(err error) error {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case mtproto.ErrBadRequest, mtproto.ErrUnauthorized, mtproto.ErrForbidden, mtproto.ErrNotFound, mtproto.ErrNotAcceptable, mtproto.ErrFlood:
			return err
		}
	}
	return mtproto.ErrUsernameInvalid
}
