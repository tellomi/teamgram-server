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
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func inputContact(clientID int64, phone, first, last string) *mtproto.InputContact {
	return &mtproto.InputContact{
		ClientId:  clientID,
		Phone:     phone,
		FirstName: first,
		LastName:  last,
	}
}

func TestPlanContactImportMatchesFormattedPhone(t *testing.T) {
	plan := PlanContactImport(10, []*mtproto.InputContact{
		inputContact(100, "+86 138-0013-8000", "Li", "Na"),
		inputContact(101, "79161234567", "Ivan", ""),
	}, []RegisteredPhone{
		{ID: 20, Phone: "8613800138000"},
		{ID: 21, Phone: "79161234567"},
	}, nil, nil)

	if len(plan.Matches) != 2 {
		t.Fatalf("matches = %#v", plan.Matches)
	}
	if plan.Matches[0].UserID != 20 || plan.Matches[0].ClientID != 100 || plan.Matches[0].Phone != "8613800138000" {
		t.Fatalf("china match = %#v", plan.Matches[0])
	}
	if plan.Matches[0].AlreadyMine || plan.Matches[0].Reverse {
		t.Fatalf("new contact was marked existing: %#v", plan.Matches[0])
	}
	if plan.Matches[1].UserID != 21 || plan.Matches[1].FirstName != "Ivan" {
		t.Fatalf("ru match = %#v", plan.Matches[1])
	}
	if len(plan.Unregistered) != 0 {
		t.Fatalf("unregistered = %#v", plan.Unregistered)
	}
	if len(plan.UpdateUserIDs) != 2 || plan.UpdateUserIDs[0] != 20 || plan.UpdateUserIDs[1] != 21 {
		t.Fatalf("update ids = %v", plan.UpdateUserIDs)
	}
}

func TestPlanContactImportMutualAndExisting(t *testing.T) {
	plan := PlanContactImport(10, []*mtproto.InputContact{
		inputContact(1, "8613800138000", "Li", "Na"),
	}, []RegisteredPhone{
		{ID: 20, Phone: "8613800138000"},
	}, []int64{20}, []int64{20})

	if len(plan.Matches) != 1 {
		t.Fatalf("matches = %#v", plan.Matches)
	}
	match := plan.Matches[0]
	if !match.AlreadyMine || !match.Reverse || match.UserID != 20 {
		t.Fatalf("match = %#v", match)
	}
	if len(plan.UpdateUserIDs) != 1 || plan.UpdateUserIDs[0] != 20 {
		t.Fatalf("update ids = %v", plan.UpdateUserIDs)
	}
}

func TestPlanContactImportSkipsSelfAndDeleted(t *testing.T) {
	plan := PlanContactImport(10, []*mtproto.InputContact{
		inputContact(1, "8613800138000", "Me", ""),
		inputContact(2, "+1 415 555 2671", "Gone", ""),
		inputContact(3, "+44 7911 123456", "New", "User"),
		inputContact(4, "", "Blank", ""),
		nil,
	}, []RegisteredPhone{
		{ID: 10, Phone: "8613800138000"},
		{ID: 30, Phone: "14155552671", Deleted: true},
	}, nil, nil)

	if len(plan.Matches) != 0 {
		t.Fatalf("matches = %#v", plan.Matches)
	}
	if len(plan.Unregistered) != 2 {
		t.Fatalf("unregistered = %#v", plan.Unregistered)
	}
	if plan.Unregistered[0].ClientID != 2 || plan.Unregistered[0].Phone != "14155552671" {
		t.Fatalf("deleted phone = %#v", plan.Unregistered[0])
	}
	if plan.Unregistered[1].ClientID != 3 || plan.Unregistered[1].Phone != "447911123456" {
		t.Fatalf("unknown phone = %#v", plan.Unregistered[1])
	}
	if len(plan.UpdateUserIDs) != 0 {
		t.Fatalf("update ids = %v", plan.UpdateUserIDs)
	}
}

func TestPlanContactImportDuplicateClientIDsOneUser(t *testing.T) {
	plan := PlanContactImport(1, []*mtproto.InputContact{
		inputContact(7, "+8613800138000", "A", ""),
		inputContact(8, "8613800138000", "B", ""),
	}, []RegisteredPhone{
		{ID: 20, Phone: "8613800138000"},
	}, nil, []int64{20})

	if len(plan.Matches) != 2 {
		t.Fatalf("matches = %#v", plan.Matches)
	}
	if plan.Matches[0].ClientID != 7 || plan.Matches[1].ClientID != 8 {
		t.Fatalf("client ids = %d %d", plan.Matches[0].ClientID, plan.Matches[1].ClientID)
	}
	if !plan.Matches[0].Reverse || !plan.Matches[1].Reverse {
		t.Fatal("reverse contact was not marked mutual")
	}
	if len(plan.UpdateUserIDs) != 1 || plan.UpdateUserIDs[0] != 20 {
		t.Fatalf("update ids = %v", plan.UpdateUserIDs)
	}
}

func TestPlanContactImportPrefersLiveUser(t *testing.T) {
	plan := PlanContactImport(1, []*mtproto.InputContact{
		inputContact(1, "8613800138000", "Li", ""),
	}, []RegisteredPhone{
		{ID: 30, Phone: "8613800138000", Deleted: true},
		{ID: 20, Phone: "+86 138 0013 8000"},
	}, nil, nil)

	if len(plan.Matches) != 1 || plan.Matches[0].UserID != 20 {
		t.Fatalf("matches = %#v", plan.Matches)
	}
}
