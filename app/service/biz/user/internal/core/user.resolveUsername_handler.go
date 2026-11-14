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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserResolveUsername
// user.resolveUsername username:string = Peer;
func (c *UserCore) UserResolveUsername(in *user.TLUserResolveUsername) (*mtproto.Peer, error) {
	var (
		err      error
		username = in.Username
	)

	switch username {
	case "gif":
	case "pic":
	case "bing":
	case "venue":
	default:
		if !user.CheckUsernameInvalid(username) {
			err = mtproto.ErrUsernameInvalid
			return nil, err
		}
	}

	usernameDO, err := c.svcCtx.Dao.UsernameDAO.SelectByUsername(c.ctx, username)
	if err != nil {
		c.Logger.Errorf("username.resolveUsername - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	} else if usernameDO == nil {
		c.Logger.Errorf("username.resolveUsername - error: %v", err)
		err = mtproto.ErrUsernameNotOccupied
		return nil, err
	}

	peer, err := peerFromUsernameRow(usernameDO)
	if err != nil {
		c.Logger.Errorf("username.resolveUsername - error: %v", err)
		return nil, err
	}
	return peer, nil
}

// peerFromUsernameRow is contacts.resolveUsername's table lookup.
// A deactivated or deleted name stays owned, but it does not open.
func peerFromUsernameRow(do *dataobject.UsernameDO) (*mtproto.Peer, error) {
	if do == nil || do.Deleted || !do.Active {
		return nil, mtproto.ErrUsernameNotOccupied
	}
	switch do.PeerType {
	case mtproto.PEER_USER:
		return mtproto.MakePeerUser(do.PeerId), nil
	case mtproto.PEER_CHANNEL:
		return mtproto.MakePeerChannel(do.PeerId), nil
	default:
		return nil, mtproto.ErrUsernameNotOccupied
	}
}
