// Copyright 2025 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserToggleUsername
// user.toggleUsername peer_type:int peer_id:long username:string active:Bool = Bool;
func (c *UserCore) UserToggleUsername(in *user.TLUserToggleUsername) (*mtproto.Bool, error) {
	name := strings.TrimSpace(in.GetUsername())
	active := mtproto.FromBool(in.GetActive())
	_, _ = c.svcCtx.Dao.UsernameDAO.Update(
		c.ctx,
		map[string]interface{}{
			"active": active,
		},
		name)
	// The public users.username column is what some readers still show.
	// Deactivating that name clears it. Activating it puts the name back
	// when the account has no other public username.
	if in.GetPeerType() == mtproto.PEER_USER && in.GetPeerId() != 0 && c.svcCtx.Dao != nil {
		me, _ := c.svcCtx.Dao.UsersDAO.SelectById(c.ctx, in.GetPeerId())
		if me != nil && strings.EqualFold(me.Username, name) && !active {
			c.svcCtx.Dao.UpdateUserUsername(c.ctx, in.GetPeerId(), "")
		}
		if me != nil && active && me.Username == "" {
			c.svcCtx.Dao.UpdateUserUsername(c.ctx, in.GetPeerId(), name)
		}
	}
	return mtproto.BoolTrue, nil
}
