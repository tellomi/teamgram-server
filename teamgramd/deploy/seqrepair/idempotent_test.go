// Copyright 2026 Teamgram Authors
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

package seqrepair

import (
	"strconv"
	"strings"
	"testing"
)

// Production shape after the id-counter restart: duplicate (user_id, pts)
// groups, one lost new message, an edited message, and a deleted message.
// Rows inserted after the restart keep the pts they were given once the
// counter was put back. The first plan repairs only the lost message and the
// duplicate pts. The second plan is empty, including the dry-run text.
func TestApplyTwiceLeavesEditsDeletesAndContinuation(t *testing.T) {
	const (
		alice = int64(1001)
		bob   = int64(1002)
	)
	snap := idempotentFixture(alice, bob)

	dups := DuplicatePts(snap.Pts)
	if len(dups) != 2 {
		t.Fatalf("duplicate pts groups = %v, want 2", dups)
	}

	plan := BuildPlan(snap, Counters{})
	if plan.Rebuilt != 1 {
		t.Fatalf("rebuilt = %d, want 1 lost message\ninserts=%+v", plan.Rebuilt, plan.Inserts)
	}
	if plan.Inserts[0].Message != "lost-one" || plan.Inserts[0].UserID != alice {
		t.Fatalf("insert = %+v, want alice lost-one", plan.Inserts[0])
	}
	if plan.Inserts[0].BoxID != 22 {
		t.Fatalf("lost box = %d, want 22 (above max 21)", plan.Inserts[0].BoxID)
	}
	for _, m := range plan.Inserts {
		switch m.Message {
		case "大郎请喝药", "大郎请喝药 1", "@mke123456", "@mke123456 2", "gone":
			t.Fatalf("inserted %q; edits and deletes are not lost messages", m.Message)
		}
	}
	if plan.PtsMoved != 5 {
		t.Fatalf("pts moved = %d, want the 5 reset rows (not the continuation)", plan.PtsMoved)
	}
	if len(plan.Dialogs) != 1 || plan.Dialogs[0].TopMessage != 22 {
		t.Fatalf("dialogs = %+v, want the lost-message dialog top 22", plan.Dialogs)
	}

	after := plan.Apply(snap)
	if dups = DuplicatePts(after.Pts); len(dups) != 0 {
		t.Fatalf("duplicate pts left: %v", dups)
	}
	for _, row := range after.Pts {
		switch row.ID {
		case 9:
			if row.Pts != 41 {
				t.Fatalf("continuation pts id 9 = %d, want 41", row.Pts)
			}
		case 10:
			if row.Pts != 42 {
				t.Fatalf("continuation pts id 10 = %d, want 42", row.Pts)
			}
		}
	}
	if n := countText(after.Messages, "大郎请喝药 1"); n != 1 {
		t.Fatalf("original edit text count = %d, want 1", n)
	}
	if n := countText(after.Messages, "大郎请喝药"); n != 1 {
		t.Fatalf("edited text count = %d, want the stored row only", n)
	}
	if n := countText(after.Messages, "@mke123456 2"); n != 1 {
		t.Fatalf("second original count = %d, want 1", n)
	}
	if n := countText(after.Messages, "@mke123456"); n != 0 {
		t.Fatalf("edit copy count = %d, want 0", n)
	}
	if n := countText(after.Messages, "gone"); n != 1 {
		t.Fatalf("deleted message count = %d, want the original row only", n)
	}
	for _, m := range after.Messages {
		if m.Message == "gone" && !m.Deleted {
			t.Fatal("deleted message was restored")
		}
	}

	again := BuildPlan(after, Counters{})
	if again.Rebuilt != 0 || again.PtsMoved != 0 || len(again.Dialogs) != 0 {
		t.Fatalf("second plan rebuilt %d pts %d dialogs %d", again.Rebuilt, again.PtsMoved, len(again.Dialogs))
	}
	dry := Report{Plan: again, DryRun: true, Snap: after}.Format("dry-run")
	if !strings.Contains(dry, "nothing to repair") {
		t.Fatalf("dry-run:\n%s", dry)
	}
	applied := Report{Plan: again, Snap: after}.Format("applied")
	if !strings.Contains(applied, "nothing to repair") {
		t.Fatalf("second apply:\n%s", applied)
	}
}

func idempotentFixture(alice, bob int64) Snapshot {
	old := int64(1000)
	return Snapshot{
		Messages: []Message{
			{UserID: alice, BoxID: 1, SenderID: alice, PeerType: peerUser, PeerID: bob, Message: "old", Date2: old},
			{UserID: alice, BoxID: 10, SenderID: alice, PeerType: peerUser, PeerID: bob, Message: "大郎请喝药 1", Date2: 1500},
			{UserID: alice, BoxID: 11, SenderID: alice, PeerType: peerUser, PeerID: bob, Message: "@mke123456 2", Date2: 1510},
			{UserID: alice, BoxID: 12, SenderID: alice, PeerType: peerUser, PeerID: bob, Message: "gone", Date2: 1600, Deleted: true},
			// Edited in place. The original updateNewMessage still carries the
			// pre-edit text and sits on the continuation, after the restart.
			{UserID: alice, BoxID: 20, SenderID: alice, PeerType: peerUser, PeerID: bob, Message: "大郎请喝药", Date2: 1700},
			{UserID: alice, BoxID: 21, SenderID: alice, PeerType: peerUser, PeerID: bob, Message: "after", Date2: 1800},
		},
		Dialogs: []Dialog{
			{UserID: alice, PeerType: peerUser, PeerID: bob, TopMessage: 21, ReadInbox: 21, ReadOutbox: 21, UnreadCount: 0, Date2: 1800},
		},
		Pts: []PtsRow{
			{ID: 1, UserID: alice, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: old, UpdateData: newMessageJSON(alice, bob, peerUser, 1, old, "old", true)},
			{ID: 2, UserID: alice, Pts: 9, PtsCount: 1, UpdateType: 4, Date2: 1100, UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":9}`},
			{ID: 3, UserID: alice, Pts: 40, PtsCount: 1, UpdateType: 4, Date2: 1400, UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":40}`},
			// Lost send. Box 1 is already the old message, so the insert was dropped.
			{ID: 4, UserID: alice, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 2000, UpdateData: newMessageJSON(alice, bob, peerUser, 1, 2000, "lost-one", true)},
			// Edit stored with update_type 1 (the column the old detector treated as a new message).
			{ID: 5, UserID: alice, Pts: 9, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 2100, UpdateData: messageUpdateJSON(updateEditMessage, alice, bob, peerUser, 10, 1500, "大郎请喝药", true)},
			{ID: 6, UserID: alice, Pts: 2, PtsCount: 1, UpdateType: ptsEditMessage, Date2: 2110, UpdateData: messageUpdateJSON(updateEditMessage, alice, bob, peerUser, 11, 1510, "@mke123456", true)},
			{ID: 7, UserID: alice, Pts: 4, PtsCount: 1, UpdateType: ptsDeleteMessages, Date2: 2200, UpdateData: deleteMessagesJSON(4, 12)},
			{ID: 8, UserID: alice, Pts: 5, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 1600, UpdateData: newMessageJSON(alice, bob, peerUser, 12, 1600, "gone", true)},
			// Continuation after the counter was restored. Text no longer matches the edited row.
			{ID: 9, UserID: alice, Pts: 41, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 1700, UpdateData: newMessageJSON(alice, bob, peerUser, 20, 1700, "大郎请喝药 1", true)},
			{ID: 10, UserID: alice, Pts: 42, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 1800, UpdateData: newMessageJSON(alice, bob, peerUser, 21, 1800, "after", true)},
		},
	}
}

func messageUpdateJSON(predicate string, from, peer int64, peerType, box int32, date int64, text string, out bool) string {
	raw := newMessageJSON(from, peer, peerType, box, date, text, out)
	return strings.Replace(raw, `"predicate_name":"`+updateNewMessage+`"`, `"predicate_name":"`+predicate+`"`, 1)
}

func deleteMessagesJSON(pts int32, ids ...int32) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(int64(id), 10)
	}
	return `{"predicate_name":"` + updateDeleteMessages + `","pts_INT32":` + strconv.FormatInt(int64(pts), 10) + `,"pts_count":` + strconv.Itoa(len(ids)) + `,"messages":[` + strings.Join(parts, ",") + `]}`
}

func countText(msgs []Message, text string) int {
	n := 0
	for _, m := range msgs {
		if m.Message == text {
			n++
		}
	}
	return n
}
