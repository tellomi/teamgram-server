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
	"encoding/json"
	"testing"
	"time"
)

// The production incident: Redis was flushed, box ids restarted at 1, and
// seven sends (six in the erel<->mke private chat, one in chat 120034,
// 02:11-03:09 UTC+8) collided with rows from 22:11-22:13 and were dropped.
// Erel's stored max box id is 51, so rebuilt ids start at 52. Mke's max is
// 40, so rebuilt ids start at 41. top_message follows those new ids.
// The pre-reset tops (27 and 18) are not the result.
func TestIncidentRebuildsSwallowedMessagesAndPts(t *testing.T) {
	const (
		erel = int64(1001)
		mke  = int64(1002)
		chat = int64(120034)
	)
	loc := time.FixedZone("CST", 8*3600)
	old := time.Date(2026, 10, 29, 22, 12, 0, 0, loc).Unix()
	privTimes := []int64{
		time.Date(2026, 10, 30, 2, 12, 0, 0, loc).Unix(),
		time.Date(2026, 10, 30, 2, 24, 0, 0, loc).Unix(),
		time.Date(2026, 10, 30, 2, 36, 0, 0, loc).Unix(),
		time.Date(2026, 10, 30, 2, 48, 0, 0, loc).Unix(),
		time.Date(2026, 10, 30, 3, 0, 0, 0, loc).Unix(),
		time.Date(2026, 10, 30, 3, 6, 0, 0, loc).Unix(),
	}
	groupAt := time.Date(2026, 10, 30, 3, 9, 0, 0, loc).Unix()

	var snap Snapshot
	// Collided slots 1..7 plus a higher id so the next box id is 52 / 41.
	for box := int32(1); box <= 7; box++ {
		snap.Messages = append(snap.Messages,
			Message{UserID: erel, BoxID: box, SenderID: erel, PeerType: peerUser, PeerID: mke, Message: "old", Date2: old},
			Message{UserID: mke, BoxID: box, SenderID: mke, PeerType: peerUser, PeerID: erel, Message: "old", Date2: old},
		)
	}
	snap.Messages = append(snap.Messages,
		Message{UserID: erel, BoxID: 27, SenderID: erel, PeerType: peerUser, PeerID: mke, Message: "pre-top", Date2: old},
		Message{UserID: erel, BoxID: 51, SenderID: erel, PeerType: peerUser, PeerID: 9, Message: "other", Date2: old},
		Message{UserID: mke, BoxID: 18, SenderID: mke, PeerType: peerUser, PeerID: erel, Message: "pre-top", Date2: old},
		Message{UserID: mke, BoxID: 40, SenderID: mke, PeerType: peerUser, PeerID: 9, Message: "other", Date2: old},
	)
	snap.Dialogs = []Dialog{
		{UserID: erel, PeerType: peerUser, PeerID: mke, TopMessage: 27, ReadInbox: 27, ReadOutbox: 5000, UnreadCount: 2, Date2: old},
		{UserID: mke, PeerType: peerUser, PeerID: erel, TopMessage: 18, ReadInbox: 18, ReadOutbox: 18, UnreadCount: 1, Date2: old},
		{UserID: erel, PeerType: peerChat, PeerID: chat, TopMessage: 0, ReadInbox: 0, ReadOutbox: 0, UnreadCount: 4, Date2: old},
		{UserID: mke, PeerType: peerChat, PeerID: chat, TopMessage: 0, ReadInbox: 0, ReadOutbox: 0, UnreadCount: 0, Date2: old},
	}

	var nextID int64 = 1
	addPts := func(user int64, pts int32, date int64, data string) {
		snap.Pts = append(snap.Pts, PtsRow{
			ID: nextID, UserID: user, Pts: pts, PtsCount: 1, UpdateType: ptsNewMessage,
			UpdateData: data, Date2: date,
		})
		nextID++
	}
	// Original pts 1..7, then a high water mark the reset fell behind.
	for pts := int32(1); pts <= 7; pts++ {
		addPts(erel, pts, old, newMessageJSON(erel, mke, peerUser, pts, old, "old", true))
		addPts(mke, pts, old, newMessageJSON(mke, erel, peerUser, pts, old, "old", true))
	}
	addPts(erel, 200, old, `{"predicate_name":"updateReadHistoryInbox","pts_INT32":200}`)
	addPts(mke, 150, old, `{"predicate_name":"updateReadHistoryInbox","pts_INT32":150}`)

	// Post-reset duplicates. Box ids 1..7 are the ones ON DUPLICATE KEY swallowed.
	for i, at := range privTimes {
		text := "p" + string(rune('1'+i))
		var sender, recv int64
		if i%2 == 0 {
			sender, recv = erel, mke
		} else {
			sender, recv = mke, erel
		}
		box := int32(i + 1)
		addPts(sender, box, at, newMessageJSON(sender, recv, peerUser, box, at, text, true))
		addPts(recv, box, at, newMessageJSON(sender, sender, peerUser, box, at, text, false))
	}
	addPts(erel, 7, groupAt, newMessageJSON(erel, chat, peerChat, 7, groupAt, "group", true))
	addPts(mke, 7, groupAt, newMessageJSON(erel, chat, peerChat, 7, groupAt, "group", false))

	plan := BuildPlan(snap, Counters{})
	if plan.Rebuilt != 14 {
		t.Fatalf("rebuilt rows = %d, want 14 (6 private x 2 boxes + 1 group x 2 boxes)", plan.Rebuilt)
	}

	erelPriv := boxes(plan.Inserts, erel, peerUser, mke)
	mkePriv := boxes(plan.Inserts, mke, peerUser, erel)
	if len(erelPriv) != 6 || len(mkePriv) != 6 {
		t.Fatalf("private boxes erel=%v mke=%v, want 6 each", erelPriv, mkePriv)
	}
	if erelPriv[0] != 52 {
		t.Fatalf("erel first rebuilt id = %d, want 52 (above max 51)", erelPriv[0])
	}
	if mkePriv[0] != 41 {
		t.Fatalf("mke first rebuilt id = %d, want 41 (above max 40)", mkePriv[0])
	}
	if !strictlyIncreasing(erelPriv) || !strictlyIncreasing(mkePriv) {
		t.Fatalf("box ids must follow send order: erel %v mke %v", erelPriv, mkePriv)
	}

	var groupErel, groupMke int32
	var shared int64
	for _, m := range plan.Inserts {
		if m.PeerType == peerChat && m.PeerID == chat {
			if m.UserID == erel {
				groupErel = m.BoxID
			}
			if m.UserID == mke {
				groupMke = m.BoxID
			}
			if shared == 0 {
				shared = m.DialogMessageID
			} else if m.DialogMessageID != shared {
				t.Fatalf("group copies do not share dialog_message_id")
			}
		}
	}
	if groupErel <= erelPriv[len(erelPriv)-1] {
		t.Fatalf("group message id %d should be above erel's last private id %d", groupErel, erelPriv[len(erelPriv)-1])
	}
	if groupMke <= mkePriv[len(mkePriv)-1] {
		t.Fatalf("group message id %d should be above mke's last private id %d", groupMke, mkePriv[len(mkePriv)-1])
	}

	pairIDs(t, plan.Inserts)

	after := plan.Apply(snap)
	erelDlg := dialogOf(after, erel, peerUser, mke)
	mkeDlg := dialogOf(after, mke, peerUser, erel)
	if erelDlg.TopMessage == 27 || erelDlg.TopMessage != erelPriv[len(erelPriv)-1] {
		t.Fatalf("erel top_message = %d, want newest private id %d (not the pre-reset 27)", erelDlg.TopMessage, erelPriv[len(erelPriv)-1])
	}
	if mkeDlg.TopMessage == 18 || mkeDlg.TopMessage != mkePriv[len(mkePriv)-1] {
		t.Fatalf("mke top_message = %d, want newest private id %d (not the pre-reset 18)", mkeDlg.TopMessage, mkePriv[len(mkePriv)-1])
	}
	groupDlg := dialogOf(after, erel, peerChat, chat)
	if groupDlg.TopMessage != groupErel {
		t.Fatalf("chat 120034 top_message = %d, want %d", groupDlg.TopMessage, groupErel)
	}

	// 27/18 stay the read cursors. 5000 covered the new low ids and is pulled
	// back to the last real message so rebuilt sends stay unread.
	if erelDlg.ReadInbox != 27 {
		t.Fatalf("erel read_inbox = %d, want 27", erelDlg.ReadInbox)
	}
	if erelDlg.ReadOutbox != 27 {
		t.Fatalf("erel read_outbox = %d, want 27 (clamped below rebuilt ids, not left at 5000)", erelDlg.ReadOutbox)
	}
	if erelDlg.ReadOutbox >= erelPriv[0] || erelDlg.ReadInbox >= erelDlg.TopMessage {
		t.Fatalf("rebuilt erel messages would still show as read: inbox %d outbox %d top %d", erelDlg.ReadInbox, erelDlg.ReadOutbox, erelDlg.TopMessage)
	}
	if mkeDlg.ReadInbox != 18 || mkeDlg.ReadOutbox != 18 {
		t.Fatalf("mke read cursors = %d/%d, want 18/18", mkeDlg.ReadInbox, mkeDlg.ReadOutbox)
	}
	// Three of the six private messages are incoming. Erel's own three, and the
	// group message he sent, must not add unread. Stored 2 stays plus the 3.
	if erelDlg.UnreadCount != 5 {
		t.Fatalf("erel private unread = %d, want 5 (stored 2 plus 3 incoming)", erelDlg.UnreadCount)
	}
	erelGroup := dialogOf(after, erel, peerChat, chat)
	if erelGroup.UnreadCount != 4 {
		t.Fatalf("erel group unread = %d, want 4 (own message does not add unread)", erelGroup.UnreadCount)
	}
	if mkeDlg.UnreadCount != 4 {
		t.Fatalf("mke private unread = %d, want 4 (stored 1 plus 3 incoming)", mkeDlg.UnreadCount)
	}

	if dups := DuplicatePts(after.Pts); len(dups) != 0 {
		t.Fatalf("duplicate (user_id, pts) left: %v", dups)
	}
	// New pts sit contiguous above the previous maximum.
	if !ptsAbove(after.Pts, erel, 200, 7) {
		t.Fatalf("erel post-reset pts were not moved above 200")
	}
	if !ptsAbove(after.Pts, mke, 150, 7) {
		t.Fatalf("mke post-reset pts were not moved above 150")
	}

	again := BuildPlan(after, Counters{})
	if again.Rebuilt != 0 || again.PtsMoved != 0 || len(again.Dialogs) != 0 {
		t.Fatalf("second plan is not empty: rebuilt %d pts %d dialogs %d", again.Rebuilt, again.PtsMoved, len(again.Dialogs))
	}
}

func newMessageJSON(from, peer int64, peerType, box int32, date int64, text string, out bool) string {
	peerObj := map[string]any{"predicate_name": "peerUser", "user_id": peer}
	if peerType == peerChat {
		peerObj = map[string]any{"predicate_name": "peerChat", "chat_id": peer}
	}
	doc := map[string]any{
		"predicate_name": updateNewMessage,
		"pts_INT32":      box,
		"pts_count":      1,
		"message_MESSAGE": map[string]any{
			"predicate_name": "message",
			"id":             box,
			"date":           date,
			"message":        text,
			"out":            out,
			"from_id":        map[string]any{"predicate_name": "peerUser", "user_id": from},
			"peer_id":        peerObj,
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func boxes(msgs []Message, user int64, peerType int32, peer int64) []int32 {
	var ids []int32
	for _, m := range msgs {
		if m.UserID == user && m.PeerType == peerType && m.PeerID == peer {
			ids = append(ids, m.BoxID)
		}
	}
	return ids
}

func strictlyIncreasing(ids []int32) bool {
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			return false
		}
	}
	return len(ids) > 0
}

func pairIDs(t *testing.T, msgs []Message) {
	t.Helper()
	type key struct {
		text string
		date int64
	}
	seen := map[key]int64{}
	for _, m := range msgs {
		if m.PeerType != peerUser {
			continue
		}
		k := key{m.Message, m.Date2}
		if id, ok := seen[k]; ok && id != m.DialogMessageID {
			t.Fatalf("private message %q has two dialog_message_ids", m.Message)
		}
		seen[k] = m.DialogMessageID
	}
}

func dialogOf(s Snapshot, user int64, peerType int32, peer int64) Dialog {
	for _, d := range s.Dialogs {
		if d.UserID == user && d.PeerType == peerType && d.PeerID == peer {
			return d
		}
	}
	return Dialog{}
}

func ptsAbove(rows []PtsRow, user int64, floor int32, want int) bool {
	var got []int32
	for _, row := range rows {
		if row.UserID == user && row.Pts > floor {
			got = append(got, row.Pts)
		}
	}
	if len(got) != want {
		return false
	}
	sortInts(got)
	for i := range got {
		if got[i] != floor+int32(i)+1 {
			return false
		}
	}
	return true
}

func TestUnreadIgnoresOutgoingAndDoesNotRecount(t *testing.T) {
	const (
		erel   = int64(1001)
		mke    = int64(1002)
		member = int64(136907719)
		chat   = int64(120034)
	)
	old := int64(100)
	at := int64(200)
	snap := Snapshot{
		Messages: []Message{
			{UserID: erel, BoxID: 1052, SenderID: erel, PeerType: peerUser, PeerID: mke, Message: "later", Date2: old},
		},
		Dialogs: []Dialog{
			{UserID: erel, PeerType: peerUser, PeerID: mke, TopMessage: 1052, ReadInbox: 27, ReadOutbox: 27, UnreadCount: 2},
			{UserID: erel, PeerType: peerChat, PeerID: chat, TopMessage: 10, ReadInbox: 10, UnreadCount: 4},
			// Stored badges are higher than a naive count of messages above the
			// read cursor. The dry-run must not replace 17 and 25 with that count.
			{UserID: mke, PeerType: peerChat, PeerID: chat, TopMessage: 40, ReadInbox: 20, UnreadCount: 17},
			{UserID: member, PeerType: peerChat, PeerID: chat, TopMessage: 40, ReadInbox: 20, UnreadCount: 25},
			{UserID: member, PeerType: peerUser, PeerID: erel, TopMessage: 8, ReadInbox: 8, UnreadCount: 3},
		},
	}
	for box := int32(21); box <= 29; box++ {
		snap.Messages = append(snap.Messages,
			Message{UserID: mke, BoxID: box, SenderID: erel, PeerType: peerChat, PeerID: chat, Message: "hist", Date2: old},
			Message{UserID: member, BoxID: box, SenderID: erel, PeerType: peerChat, PeerID: chat, Message: "hist", Date2: old},
		)
	}
	// Old pts so the new pts=1 rows are the post-reset duplicates.
	snap.Pts = append(snap.Pts,
		PtsRow{ID: 1, UserID: erel, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: old, UpdateData: newMessageJSON(erel, mke, peerUser, 1, old, "old", true)},
		PtsRow{ID: 2, UserID: erel, Pts: 1153, PtsCount: 1, UpdateType: ptsNewMessage, Date2: old, UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":1153}`},
		PtsRow{ID: 3, UserID: mke, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: old, UpdateData: newMessageJSON(mke, chat, peerChat, 1, old, "old", true)},
		PtsRow{ID: 4, UserID: mke, Pts: 80, PtsCount: 1, Date2: old, UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":80}`},
		PtsRow{ID: 5, UserID: member, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: old, UpdateData: newMessageJSON(member, chat, peerChat, 1, old, "old", true)},
		PtsRow{ID: 6, UserID: member, Pts: 90, PtsCount: 1, Date2: old, UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":90}`},
	)
	// Six outgoing private messages from erel, plus the group message fanout.
	for i := 0; i < 6; i++ {
		text := "out" + string(rune('a'+i))
		snap.Pts = append(snap.Pts, PtsRow{
			ID: 100 + int64(i), UserID: erel, Pts: int32(i + 1), PtsCount: 1, UpdateType: ptsNewMessage, Date2: at + int64(i),
			UpdateData: newMessageJSON(erel, mke, peerUser, int32(i+1), at+int64(i), text, true),
		})
	}
	snap.Pts = append(snap.Pts,
		PtsRow{ID: 200, UserID: erel, Pts: 7, PtsCount: 1, UpdateType: ptsNewMessage, Date2: at + 10, UpdateData: newMessageJSON(erel, chat, peerChat, 7, at+10, "group", true)},
		PtsRow{ID: 201, UserID: mke, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: at + 10, UpdateData: newMessageJSON(erel, chat, peerChat, 1, at+10, "group", false)},
		PtsRow{ID: 202, UserID: member, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: at + 10, UpdateData: newMessageJSON(erel, chat, peerChat, 1, at+10, "group", false)},
	)

	plan := BuildPlan(snap, Counters{
		Box: map[int64]int64{erel: 2052, mke: 1400, member: 900},
		Pts: map[int64]int64{erel: 2153, mke: 1800, member: 1900},
	})

	erelPriv := boxes(plan.Inserts, erel, peerUser, mke)
	if len(erelPriv) != 6 || erelPriv[0] != 2053 {
		t.Fatalf("erel private boxes = %v, want 2053.. (above counter 2052, not 52)", erelPriv)
	}
	if plan.BoxTo[erel] != 2059 {
		t.Fatalf("erel box counter target = %d, want 2059 (2053 plus 6 private and the group message)", plan.BoxTo[erel])
	}
	if plan.PtsTo[erel] != 2160 {
		t.Fatalf("erel pts target = %d, want 2160 (above live counter 2153)", plan.PtsTo[erel])
	}

	erelDlg := findPlanDialog(plan, erel, peerUser, mke)
	if erelDlg.UnreadCount != 2 {
		t.Fatalf("erel private unread = %d, want 2 (six sends are his own)", erelDlg.UnreadCount)
	}
	erelGroup := findPlanDialog(plan, erel, peerChat, chat)
	if erelGroup.UnreadCount != 4 {
		t.Fatalf("erel group unread = %d, want 4", erelGroup.UnreadCount)
	}
	mkeGroup := findPlanDialog(plan, mke, peerChat, chat)
	if mkeGroup.UnreadCount != 18 {
		t.Fatalf("mke group unread = %d, want 18 (17 plus the one incoming), not a recount of 9", mkeGroup.UnreadCount)
	}
	memberGroup := findPlanDialog(plan, member, peerChat, chat)
	if memberGroup.UnreadCount != 26 {
		t.Fatalf("member group unread = %d, want 26 (25 plus one incoming), not 9", memberGroup.UnreadCount)
	}
	if findPlanDialog(plan, member, peerUser, erel).UserID != 0 {
		t.Fatal("private dialog with no swallowed message must not be rewritten")
	}
}

func findPlanDialog(p Plan, user int64, peerType int32, peer int64) Dialog {
	for _, d := range p.Dialogs {
		if d.UserID == user && d.PeerType == peerType && d.PeerID == peer {
			return d
		}
	}
	return Dialog{}
}

func TestMessageTableFor(t *testing.T) {
	if got := messageTableFor([]string{"messages"}, 1001); got != "messages" {
		t.Fatalf("unsharded table = %s", got)
	}
	if got := messageTableFor([]string{"messages_0", "messages_1", "messages_2", "messages_3"}, 6); got != "messages_2" {
		t.Fatalf("shard = %s, want messages_2", got)
	}
}

func sortInts(v []int32) {
	for i := 0; i < len(v); i++ {
		for j := i + 1; j < len(v); j++ {
			if v[j] < v[i] {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}
