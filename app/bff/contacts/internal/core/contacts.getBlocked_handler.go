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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsGetBlocked
// contacts.getBlocked#9a868f80 flags:# my_stories_from:flags.0?true offset:int limit:int = contacts.Blocked;
func (c *ContactsCore) ContactsGetBlocked(in *mtproto.TLContactsGetBlocked) (*mtproto.Contacts_Blocked, error) {
	if in.GetMyStoriesFrom() {
		return c.storyBlockedPage(in)
	}
	offset, limit := blockedListWindow(in.GetOffset(), in.GetLimit())
	page, err := c.svcCtx.Dao.UserClient.UserGetBlockedPage(c.ctx, &userpb.TLUserGetBlockedList{
		UserId: c.MD.UserId,
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getBlocked - error: %v", err)
		return nil, err
	}
	if page == nil {
		page = mtproto.MakeTLContactsBlocked(&mtproto.Contacts_Blocked{
			Blocked: []*mtproto.PeerBlocked{},
			Chats:   []*mtproto.Chat{},
			Users:   []*mtproto.User{},
		}).To_Contacts_Blocked()
	}
	if page.Chats == nil {
		page.Chats = []*mtproto.Chat{}
	}
	if page.Blocked == nil {
		page.Blocked = []*mtproto.PeerBlocked{}
	}
	idHelper := mtproto.NewIDListHelper(c.MD.UserId)
	for _, blocked := range page.Blocked {
		idHelper.PickByPeer(blocked.GetPeerId())
	}
	idHelper.Visit(
		func(userIdList []int64) {
			users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
				Id: userIdList,
			})
			if users != nil {
				page.Users = users.GetUserListByIdList(c.MD.UserId, userIdList...)
			}
		},
		func(chatIdList []int64) {},
		func(channelIdList []int64) {},
	)
	if page.Users == nil {
		page.Users = []*mtproto.User{}
	}
	return page, nil
}

// blockedListWindow is the official offset/limit page. offset skips that many
// blocked peers. limit is the page size, default and maximum 100.
func blockedListWindow(offset, limit int32) (int32, int32) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	return offset, limit
}
