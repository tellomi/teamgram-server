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
		expires = mtproto.UserOnlinePeriod
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
		prevOnline := false
		prev, prevErr := c.svcCtx.UserClient.UserGetLastSeen(ctx, &userpb.TLUserGetLastSeen{Id: c.MD.UserId})
		if prevErr == nil && prev != nil {
			prevOnline = mtproto.PresenceOnline(now, prev.GetLastSeenAt(), prev.GetExpires())
		} else if prevErr != nil && !missingPresence(prevErr) {
			_, _ = c.svcCtx.UserClient.UserUpdateLastSeen(ctx, &userpb.TLUserUpdateLastSeen{
				Id:         c.MD.UserId,
				LastSeenAt: now,
				Expires:    expires,
			})
			return
		}
		_, _ = c.svcCtx.UserClient.UserUpdateLastSeen(ctx, &userpb.TLUserUpdateLastSeen{
			Id:         c.MD.UserId,
			LastSeenAt: now,
			Expires:    expires,
		})
		// Heartbeats that only extend an open window do not push.
		if c.svcCtx.SyncClient == nil || !shouldPushStatus(prevOnline, !offline) {
			return
		}
		ids, err := c.svcCtx.UserClient.UserGetContactIdList(ctx, &userpb.TLUserGetContactIdList{
			UserId: c.MD.UserId,
		})
		if err != nil || ids == nil {
			return
		}
		rev, err := c.svcCtx.UserClient.UserGetReverseContactIdList(ctx, &userpb.TLUserGetContactIdList{
			UserId: c.MD.UserId,
		})
		if err != nil || rev == nil {
			return
		}
		forward := ids.GetDatas()
		reverse := rev.GetDatas()
		candidates := append(append([]int64{c.MD.UserId}, forward...), reverse...)
		got, err := c.svcCtx.UserClient.UserGetMutableUsersV2(ctx, &userpb.TLUserGetMutableUsersV2{
			Id:      candidates,
			Privacy: true,
			HasTo:   true,
			To:      []int64{c.MD.UserId},
		})
		if err != nil || got == nil {
			return
		}
		me, ok := got.GetImmutableUser(c.MD.UserId)
		if !ok || me == nil {
			return
		}
		attachForwardContacts(me, forward)
		blockedRows, err := c.svcCtx.UserClient.UserBlockedEitherWay(ctx, &userpb.TLUserCheckBlockUserList{
			UserId: c.MD.UserId,
			Id:     append(append([]int64{}, forward...), reverse...),
		})
		if err != nil || blockedRows == nil {
			return
		}
		blocked := make(map[int64]struct{}, len(blockedRows.GetDatas()))
		for _, id := range blockedRows.GetDatas() {
			blocked[id] = struct{}{}
		}
		exact := pushUpdates.GetUpdate().GetStatus_USERSTATUS()
		visible := func(id int64) (bool, bool, bool) {
			viewer, found := got.GetImmutableUser(id)
			if !found || viewer == nil {
				return false, false, false
			}
			return me.CheckPrivacy(mtproto.STATUS_TIMESTAMP, id), viewer.CheckPrivacy(mtproto.STATUS_TIMESTAMP, c.MD.UserId), true
		}
		for _, row := range statusDeliveries(c.MD.UserId, exact, now, expires, now, forward, reverse, visible, blocked) {
			_, _ = c.svcCtx.SyncClient.SyncPushUpdates(ctx, &syncpb.TLSyncPushUpdates{
				UserId:  row.UserID,
				Updates: userStatusPush(c.MD.UserId, row.Status, int32(now)-1),
			})
		}
	})

	return mtproto.BoolTrue, nil
}

func missingPresence(err error) bool {
	return err != nil && strings.Contains(err.Error(), "USER_ID_INVALID")
}
