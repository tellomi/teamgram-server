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
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

// RegisteredPhone is a users row that can be matched by phone.
type RegisteredPhone struct {
	ID      int64
	Phone   string
	Deleted bool
}

// ImportMatch is one input contact that belongs to a live registered user.
type ImportMatch struct {
	ClientID    int64
	UserID      int64
	Phone       string
	FirstName   string
	LastName    string
	AlreadyMine bool
	// Reverse is true when that user already has the importer as a contact,
	// so the new link is mutual.
	Reverse bool
}

// UnregisteredImport is an input contact with no live account.
type UnregisteredImport struct {
	ClientID  int64
	Phone     string
	FirstName string
	LastName  string
}

// ImportPlan is the pure result of matching an import batch.
type ImportPlan struct {
	Matches       []ImportMatch
	Unregistered  []UnregisteredImport
	LookupPhones  []string
	UpdateUserIDs []int64
}

// PlanContactImport matches contacts to registered phones.
//
// Phones are compared by LookupKeys, so "+86 138-0013-8000" matches the
// canonical "8613800138000" stored at signup. The importer's own number and
// deleted accounts are not returned as imported users. Deleted accounts are
// kept as unregistered so a later signup on that phone can be linked.
// UpdateUserIDs is every distinct matched user so both sides can be notified.
func PlanContactImport(selfID int64, contacts []*mtproto.InputContact, registered []RegisteredPhone, myContactIDs, reverseIDs []int64) ImportPlan {
	mine := idSet(myContactIDs)
	reverse := idSet(reverseIDs)
	byPhone := indexRegisteredPhones(registered)

	plan := ImportPlan{}
	seenUser := make(map[int64]struct{})
	seenLookup := make(map[string]struct{})

	for _, contact := range contacts {
		if contact == nil || contact.Phone == "" {
			continue
		}
		for _, key := range phonenumber.LookupKeys(contact.Phone) {
			if _, ok := seenLookup[key]; ok {
				continue
			}
			seenLookup[key] = struct{}{}
			plan.LookupPhones = append(plan.LookupPhones, key)
		}

		user, ok := findRegistered(byPhone, contact.Phone)
		if ok && !user.Deleted && user.ID == selfID {
			continue
		}
		if !ok || user.Deleted || user.ID == 0 {
			plan.Unregistered = append(plan.Unregistered, UnregisteredImport{
				ClientID:  contact.ClientId,
				Phone:     canonicalImportPhone(contact.Phone),
				FirstName: contact.FirstName,
				LastName:  contact.LastName,
			})
			continue
		}

		_, already := mine[user.ID]
		_, hasReverse := reverse[user.ID]
		phone := user.Phone
		if phone == "" {
			phone = canonicalImportPhone(contact.Phone)
		}
		plan.Matches = append(plan.Matches, ImportMatch{
			ClientID:    contact.ClientId,
			UserID:      user.ID,
			Phone:       phone,
			FirstName:   contact.FirstName,
			LastName:    contact.LastName,
			AlreadyMine: already,
			Reverse:     hasReverse,
		})
		if _, dup := seenUser[user.ID]; !dup {
			seenUser[user.ID] = struct{}{}
			plan.UpdateUserIDs = append(plan.UpdateUserIDs, user.ID)
		}
	}
	if plan.Matches == nil {
		plan.Matches = []ImportMatch{}
	}
	if plan.Unregistered == nil {
		plan.Unregistered = []UnregisteredImport{}
	}
	if plan.LookupPhones == nil {
		plan.LookupPhones = []string{}
	}
	if plan.UpdateUserIDs == nil {
		plan.UpdateUserIDs = []int64{}
	}
	return plan
}

func canonicalImportPhone(phone string) string {
	if _, digits, err := phonenumber.CheckPhoneNumberInvalid(phone); err == nil && digits != "" {
		return digits
	}
	digits := phonenumber.NormalizeDigits(phone)
	if len(digits) > 4 && len(digits) >= 2 && digits[:2] == "00" {
		return digits[2:]
	}
	return digits
}

func idSet(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id != 0 {
			set[id] = struct{}{}
		}
	}
	return set
}

func indexRegisteredPhones(registered []RegisteredPhone) map[string]RegisteredPhone {
	byPhone := make(map[string]RegisteredPhone, len(registered))
	for _, user := range registered {
		if user.ID == 0 || user.Phone == "" {
			continue
		}
		for _, key := range phonenumber.LookupKeys(user.Phone) {
			prev, ok := byPhone[key]
			if ok && !prev.Deleted && user.Deleted {
				continue
			}
			byPhone[key] = user
		}
	}
	return byPhone
}

func findRegistered(byPhone map[string]RegisteredPhone, phone string) (RegisteredPhone, bool) {
	for _, key := range phonenumber.LookupKeys(phone) {
		user, ok := byPhone[key]
		if ok {
			return user, true
		}
	}
	return RegisteredPhone{}, false
}
