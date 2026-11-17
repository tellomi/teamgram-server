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
	"context"

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
	// Privacy could not be applied. Return no exact timestamps rather than
	// marking every contact online from the raw presence row.
	rows := contactStatusesFromUsers(err == nil && got != nil, got, c.MD.UserId, idList)
	rList.Datas = rows
	return rList, nil
}

// contactStatusesFromUsers builds contact statuses only when the user load
// succeeded. A failed load returns no rows: exact last seen must not be
// filled in for every contact.
func contactStatusesFromUsers(loaded bool, got *mtproto.MutableUsers, self int64, idList []int64) []*mtproto.ContactStatus {
	if !loaded || got == nil {
		return []*mtproto.ContactStatus{}
	}
	me, ok := got.GetImmutableUser(self)
	if !ok || me == nil {
		return []*mtproto.ContactStatus{}
	}
	setForwardContacts(me, idList)
	page := make([]*mtproto.ImmutableUser, 0, len(idList))
	for _, id := range idList {
		peer, ok := got.GetImmutableUser(id)
		if ok && peer != nil {
			page = append(page, peer)
		}
	}
	ctx := mtproto.PreparePrivacyRender(context.Background(), self, append(page, me))
	out := make([]*mtproto.ContactStatus, 0, len(idList))
	for _, peer := range page {
		viewed := peer.ToUnsafeUserCtx(ctx, me)
		if viewed == nil || viewed.Status == nil {
			continue
		}
		out = append(out, mtproto.MakeTLContactStatus(&mtproto.ContactStatus{
			UserId: peer.Id(),
			Status: viewed.Status,
		}).To_ContactStatus())
	}
	return out
}
