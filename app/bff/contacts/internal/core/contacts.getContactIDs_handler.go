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
	"math"
	"sort"

	"github.com/teamgram/proto/mtproto"
)

// ContactsGetContactIDs
// contacts.getContactIDs#7adc669d hash:long = Vector<int>;
//
// The hash is contacts.getContacts' hash (saved_count, then contact user ids
// in ascending order, MediaDataController.calcHash). Hash 0 always returns
// the ids. A non-zero hash that matches returns an empty vector, which the
// client keeps as "not modified".
func (c *ContactsCore) ContactsGetContactIDs(in *mtproto.TLContactsGetContactIDs) (*mtproto.Vector_Int, error) {
	contactList, err := c.contactRows()
	if err != nil {
		c.Logger.Errorf("contacts.getContactIDs - error: %v", err)
		if in.GetHash() != 0 {
			return nil, err
		}
		contactList = nil
	}
	seen := map[int64]bool{}
	contacts := make([]*mtproto.Contact, 0, len(contactList))
	for _, row := range contactList {
		if row == nil || row.GetContactUserId() == 0 || seen[row.GetContactUserId()] {
			continue
		}
		seen[row.GetContactUserId()] = true
		contacts = append(contacts, mtproto.MakeTLContact(&mtproto.Contact{
			UserId: row.GetContactUserId(),
			Mutual: mtproto.ToBool(row.GetMutualContact()),
		}).To_Contact())
	}
	saved, err := c.savedCount()
	if err != nil {
		c.Logger.Errorf("contacts.getContactIDs - saved_count: %v", err)
		if in.GetHash() != 0 {
			return nil, err
		}
	} else if in.GetHash() != 0 && in.GetHash() == contactsHash(saved, contacts) {
		return &mtproto.Vector_Int{Datas: []int32{}}, nil
	}
	ids := make([]int32, 0, len(contacts))
	for _, row := range contacts {
		id := row.GetUserId()
		if id <= 0 || id > math.MaxInt32 {
			continue
		}
		ids = append(ids, int32(id))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return &mtproto.Vector_Int{Datas: ids}, nil
}
