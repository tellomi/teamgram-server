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
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/threading"
)

// AccountUpdateStatus
// account.updateStatus#6628562c offline:Bool = Bool;
func (c *UserChannelProfilesCore) AccountUpdateStatus(in *mtproto.TLAccountUpdateStatus) (*mtproto.Bool, error) {
	var (
		offline     = mtproto.FromBool(in.GetOffline())
		now         = time.Now().Unix()
		expires     = int32(0)
		pushUpdates = mtproto.MakeTLUpdateShort(&mtproto.Updates{
			Update: mtproto.MakeTLUpdateUserStatus(&mtproto.Update{
				UserId:            c.MD.UserId,
				Status_USERSTATUS: nil,
			}).To_Update(),
			Date: int32(now) - 1,
		}).To_Updates()
	)

	if !offline {
		expires = 300
		pushUpdates.Update.Status_USERSTATUS = mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{
			Expires: int32(now) + expires,
		}).To_UserStatus()

		// online
		// s.UserFacade.UpdateUserStatus(ctx, md.UserId, now)
		// c.svcCtx.Dao.UserClient.UserUpdateLastSeen()
	} else {
		pushUpdates.Update.Status_USERSTATUS = mtproto.MakeTLUserStatusOffline(&mtproto.UserStatus{
			WasOnline: int32(now),
		}).To_UserStatus()
	}

	threading.GoSafe(func() {
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.UserClient == nil {
			return
		}
		ctx := contextx.ValueOnlyFrom(c.ctx)
		_, _ = c.svcCtx.UserClient.UserUpdateLastSeen(ctx, &userpb.TLUserUpdateLastSeen{
			Id:         c.MD.UserId,
			LastSeenAt: now,
			Expires:    expires,
		})
		if c.svcCtx.SyncClient == nil {
			return
		}
		ids, err := c.svcCtx.UserClient.UserGetContactIdList(ctx, &userpb.TLUserGetContactIdList{
			UserId: c.MD.UserId,
		})
		if err != nil || ids == nil {
			return
		}
		blocked := func(id int64) bool {
			// Either direction hides the status: the contact blocked this
			// user, or this user blocked the contact.
			one, err1 := c.svcCtx.UserClient.UserBlockedByUser(ctx, &userpb.TLUserBlockedByUser{
				UserId: id, PeerUserId: c.MD.UserId,
			})
			two, err2 := c.svcCtx.UserClient.UserBlockedByUser(ctx, &userpb.TLUserBlockedByUser{
				UserId: c.MD.UserId, PeerUserId: id,
			})
			return (err1 == nil && mtproto.FromBool(one)) || (err2 == nil && mtproto.FromBool(two))
		}
		deliverUserStatus(c.MD.UserId, pushUpdates, ids.GetDatas(), blocked,
			func(id int64, up *mtproto.Updates) {
				_, _ = c.svcCtx.SyncClient.SyncPushUpdates(ctx, &syncpb.TLSyncPushUpdates{
					UserId:  id,
					Updates: up,
				})
			},
		)
	})

	return mtproto.BoolTrue, nil
}
