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
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
)

// ContactsGetSaved
// contacts.getSaved#82f1e39f = Vector<SavedContact>;
// https://core.telegram.org/method/contacts.getSaved
// https://core.telegram.org/api/takeout
//
// Exports the phone book (matched contacts plus numbers that never matched a
// Telegram account). The call has to be wrapped in invokeWithTakeout for a
// session opened with the contacts flag.
func (c *ContactsCore) ContactsGetSaved(in *mtproto.TLContactsGetSaved) (*mtproto.Vector_SavedContact, error) {
	_ = in
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	takeoutID := int64(0)
	if c.MD.HasTakeout() {
		takeoutID = c.MD.GetTakeoutId()
	}
	if err := accountrest.RequireContactsExport(c.MD.UserId, takeoutID); err != nil {
		c.Logger.Errorf("contacts.getSaved - error: %v", err)
		return nil, err
	}
	rows, err := c.listSaved()
	if err != nil {
		c.Logger.Errorf("contacts.getSaved - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	out := &mtproto.Vector_SavedContact{Datas: make([]*mtproto.SavedContact, 0, len(rows))}
	for _, row := range rows {
		out.Datas = append(out.Datas, dao.ToSavedContact(row))
	}
	return out, nil
}

func (c *ContactsCore) listSaved() ([]dao.SavedPhone, error) {
	store := c.savedBook()
	if store == nil {
		return nil, nil
	}
	return store.List(c.ctx, c.MD.UserId)
}
