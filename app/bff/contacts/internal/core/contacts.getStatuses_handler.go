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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsGetStatuses
// contacts.getStatuses#c4a353ee = Vector<ContactStatus>;
func (c *ContactsCore) ContactsGetStatuses(in *mtproto.TLContactsGetStatuses) (*mtproto.Vector_ContactStatus, error) {
	cList, _ := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})

	rList := &mtproto.Vector_ContactStatus{
		Datas: make([]*mtproto.ContactStatus, 0, len(cList.GetDatas())),
	}

	idList := make([]int64, 0, len(cList.GetDatas()))
	for _, id := range cList.GetDatas() {
		idList = append(idList, id.ContactUserId)
	}
	if len(idList) == 0 {
		return rList, nil
	}

	got, err := c.svcCtx.Dao.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{
		Id:      append([]int64{c.MD.UserId}, idList...),
		Privacy: true,
		HasTo:   true,
		To:      []int64{c.MD.UserId},
	})
	if err == nil && got != nil {
		if me, ok := got.GetImmutableUser(c.MD.UserId); ok && me != nil {
			setForwardContacts(me, idList)
			for _, id := range idList {
				peer, ok := got.GetImmutableUser(id)
				if !ok || peer == nil {
					continue
				}
				viewed := peer.ToUnsafeUser(me)
				if viewed == nil || viewed.Status == nil {
					continue
				}
				rList.Datas = append(rList.Datas, mtproto.MakeTLContactStatus(&mtproto.ContactStatus{
					UserId: id,
					Status: viewed.Status,
				}).To_ContactStatus())
			}
			return rList, nil
		}
	}

	// The user service was unreachable. Still honor the stored expires
	// window instead of treating a 5 minute heartbeat as 1 minute.
	lastSeenList, _ := c.svcCtx.Dao.UserGetLastSeens(c.ctx, &userpb.TLUserGetLastSeens{Id: idList})
	now := time.Now().Unix()
	for _, v := range lastSeenList.GetDatas() {
		status := mtproto.PresenceStatus(v.LastSeenAt, v.Expires, true, now)
		if status == nil {
			continue
		}
		rList.Datas = append(rList.Datas, mtproto.MakeTLContactStatus(&mtproto.ContactStatus{
			UserId: v.UserId,
			Status: status,
		}).To_ContactStatus())
	}

	return rList, nil
}
