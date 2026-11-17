// Copyright (c) 2024-present,  Teamgram Studio (https://teamgram.io).
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

package mtproto

import "context"

func (m *MutableUsers) GetUnsafeUser(selfId, id int64) (*User, error) {
	if len(m.Users) == 0 {
		return nil, ErrUserIdInvalid
	}

	if selfId == id {
		return m.GetUnsafeUserSelf(selfId)
	}

	me := m.findImmutableUser(selfId)
	to := m.findImmutableUser(id)

	if me == nil || to == nil {
		return nil, ErrUserIdInvalid
	}

	return paintImmutable(selfId, to, to.ToUnsafeUserCtx(PreparePrivacyRender(context.Background(), selfId, []*ImmutableUser{to, me}), me)), nil
}

func (m *MutableUsers) GetUnsafeUserSelf(id int64) (*User, error) {
	if len(m.Users) == 0 {
		return nil, ErrUserIdInvalid
	}

	me := m.findImmutableUser(id)
	if me == nil {
		return nil, ErrUserIdInvalid
	}

	return paintImmutable(id, me, me.ToSelfUser()), nil
}

func (m *MutableUsers) findImmutableUser(id int64) *ImmutableUser {
	for _, v := range m.Users {
		if v.Id() == id {
			return v
		}
	}

	return nil
}

func (m *MutableUsers) GetImmutableUser(id int64) (u *ImmutableUser, ok bool) {
	for _, v := range m.Users {
		if v.Id() == id {
			return v, true
		}
	}

	return nil, false
}

func (m *MutableUsers) CheckExistUser(id ...int64) bool {
	for _, id2 := range id {
		if _, ok := m.GetImmutableUser(id2); !ok {
			return false
		}
	}

	return true
}

func (m *MutableUsers) GetUserListByIdList(selfId int64, id ...int64) []*User {
	users := make([]*User, 0, len(id))

	if len(m.Users) == 0 {
		return users
	}

	me := m.findImmutableUser(selfId)
	if me == nil {
		return users
	}

	column := make(map[int64]int32, len(id))
	page := make([]*ImmutableUser, 0, len(id))
	for _, id2 := range id {
		if to := m.findImmutableUser(id2); to != nil {
			page = append(page, to)
		}
	}
	ctx := PreparePrivacyRender(context.Background(), selfId, append(page, me))
	for _, to := range page {
		users = append(users, to.ToUnsafeUserCtx(ctx, me))
		column[to.Id()] = StoryColumn(to)
	}
	PaintUserStoryRings(context.Background(), selfId, users, column)
	return users
}

func paintImmutable(viewer int64, im *ImmutableUser, user *User) *User {
	if im == nil || user == nil {
		return user
	}
	PaintUserStoryRings(context.Background(), viewer, []*User{user}, map[int64]int32{im.Id(): StoryColumn(im)})
	return user
}

func (m *MutableUsers) Visit(cb func(it *ImmutableUser)) {
	for _, v := range m.Users {
		cb(v)
	}
}

func (m *MutableUsers) VisitByMe(meId int64, cb func(me, it *ImmutableUser)) {
	var (
		me *ImmutableUser
	)

	for _, v := range m.Users {
		if v.Id() == meId {
			me = v
			break
		}
	}

	if me == nil {
		return
	}

	for _, v := range m.Users {
		cb(me, v)
	}
}

func (m *MutableUsers) Length() int {
	return len(m.Users)
}

func (m *MutableUsers) Empty() bool {
	return m == nil || len(m.Users) == 0
}
