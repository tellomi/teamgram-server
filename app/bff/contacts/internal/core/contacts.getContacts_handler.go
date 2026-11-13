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
)

/*
   Current Android (ContactsController.getContactsHash):

   private long getContactsHash(ArrayList<TLRPC.TL_contact> contacts) {
       long acc = 0;
       contacts = new ArrayList<>(contacts);
       Collections.sort(contacts, (a, b) -> Long.compare(a.user_id, b.user_id));
       int count = contacts.size();
       for (int a = -1; a < count; a++) {
           if (a == -1) {
               acc = MediaDataController.calcHash(acc, getUserConfig().contactsSavedCount);
           } else {
               acc = MediaDataController.calcHash(acc, contacts.get(a).user_id);
           }
       }
       return acc;
   }

   calcHash shifts the accumulator, then adds the id. A matching non-zero
   hash is contacts.contactsNotModified. Hash 0 always wants the full list.
   NotModified does not rewrite the phone book and does not call
   loadContacts again. A full list whose users are missing from the vector
   does: processLoadedContacts then calls loadContacts(false, 0).
*/

// ContactsGetContacts
// contacts.getContacts#5dd69e12 hash:long = contacts.Contacts;
func (c *ContactsCore) ContactsGetContacts(in *mtproto.TLContactsGetContacts) (*mtproto.Contacts_Contacts, error) {
	contactList, err := c.contactRows()
	if err != nil {
		c.Logger.Errorf("contacts.getContacts - error: %v", err)
		// Hash 0 is the first load. An empty success keeps that from spinning.
		// A client that already has a book sends a non-zero hash; an empty
		// success would replace that book with nothing.
		if in.GetHash() != 0 {
			return nil, err
		}
		contactList = nil
	}
	saved, err := c.savedCount()
	if err != nil {
		// A failed read must not be reported as saved_count 0. That value
		// is part of the hash, so the next successful read would look like
		// a contact change and the client would fetch again.
		c.Logger.Errorf("contacts.getContacts - saved_count: %v", err)
		return nil, err
	}
	seen := map[int64]bool{}
	tlContacts := make([]*mtproto.Contact, 0, len(contactList))
	for _, row := range contactList {
		if row == nil || row.GetContactUserId() == 0 || seen[row.GetContactUserId()] {
			continue
		}
		seen[row.GetContactUserId()] = true
		tlContacts = append(tlContacts, mtproto.MakeTLContact(&mtproto.Contact{
			UserId: row.GetContactUserId(),
			Mutual: mtproto.ToBool(row.GetMutualContact()),
		}).To_Contact())
	}
	if in.GetHash() != 0 && in.GetHash() == contactsHash(saved, tlContacts) {
		return mtproto.MakeTLContactsContactsNotModified(nil).To_Contacts_Contacts(), nil
	}
	if len(tlContacts) == 0 {
		return mtproto.MakeTLContactsContacts(&mtproto.Contacts_Contacts{
			Contacts:   []*mtproto.Contact{},
			SavedCount: saved,
			Users:      []*mtproto.User{},
		}).To_Contacts_Contacts(), nil
	}

	ids := make([]int64, 0, len(tlContacts))
	for _, row := range tlContacts {
		ids = append(ids, row.GetUserId())
	}
	users, err := c.contactUsers(ids)
	if err != nil {
		c.Logger.Errorf("contacts.getContacts - users: %v", err)
		return nil, err
	}
	kept, keptUsers := visibleContacts(tlContacts, users)
	if len(kept) == 0 {
		// Every contact was unresolved. An empty success would replace the
		// client's book with nothing. An error does not.
		c.Logger.Errorf("contacts.getContacts - no user for %d contacts", len(tlContacts))
		return nil, mtproto.ErrInternalServerError
	}
	// The client hashes the contacts it stored. Those are the ones that had
	// a user in the previous reply. A contact we still cannot resolve is not
	// part of that hash, so compare again after dropping it.
	if in.GetHash() != 0 && in.GetHash() == contactsHash(saved, kept) {
		return mtproto.MakeTLContactsContactsNotModified(nil).To_Contacts_Contacts(), nil
	}
	return mtproto.MakeTLContactsContacts(&mtproto.Contacts_Contacts{
		Contacts:   kept,
		SavedCount: saved,
		Users:      keptUsers,
	}).To_Contacts_Contacts(), nil
}

// savedCount is the number of phone-book rows that have no Telegram account.
// contacts.resetSaved deletes those rows. The error is returned to the
// caller; it is not turned into 0.
func (c *ContactsCore) savedCount() (int32, error) {
	store := c.savedBook()
	if store == nil || c.MD == nil {
		return 0, nil
	}
	return store.UnregisteredCount(c.ctx, c.MD.UserId)
}
