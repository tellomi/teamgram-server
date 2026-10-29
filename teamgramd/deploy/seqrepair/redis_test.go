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
	"context"
	"strings"
	"testing"
)

func TestReserveThenNextSendCannotCollide(t *testing.T) {
	const erel = int64(1001)
	ctx := context.Background()
	seq := NewMemSeq(map[string]string{
		boxKey(erel): "2052",
		ptsKey(erel): "2153",
	})
	snap := swallowedOne(erel)
	read, err := readCounters(ctx, seq, snap)
	if err != nil {
		t.Fatal(err)
	}
	preview := BuildPlan(snap, read)
	if len(preview.Inserts) != 1 || preview.Inserts[0].BoxID != 2053 {
		t.Fatalf("preview box = %+v, want a single id 2053", preview.Inserts)
	}
	plan, err := reserveAndPlan(ctx, seq, snap, read, preview)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Inserts[0].BoxID != 2053 {
		t.Fatalf("reserved box = %d, want 2053", plan.Inserts[0].BoxID)
	}
	if seq.Value(boxKey(erel)) != "2053" {
		t.Fatalf("box counter = %s, want 2053", seq.Value(boxKey(erel)))
	}
	if seq.Value(ptsKey(erel)) != "2154" {
		t.Fatalf("pts counter = %s, want 2154", seq.Value(ptsKey(erel)))
	}
	next, _, err := seq.Reserve(ctx, boxKey(erel), 1052, 1)
	if err != nil {
		t.Fatal(err)
	}
	if next != 2054 {
		t.Fatalf("next send got %d, want 2054", next)
	}
}

func TestDryRunFormatShowsCounterAndIncomingUnread(t *testing.T) {
	const (
		erel = int64(1001)
		mke  = int64(1002)
	)
	snap := swallowedOne(erel)
	snap.Dialogs = []Dialog{
		{UserID: erel, PeerType: peerUser, PeerID: mke, TopMessage: 1052, ReadInbox: 1052, ReadOutbox: 1052, UnreadCount: 0},
		{UserID: mke, PeerType: peerUser, PeerID: erel, TopMessage: 40, ReadInbox: 40, UnreadCount: 17},
	}
	// Mke's copy of the same send is incoming.
	snap.Pts = append(snap.Pts, PtsRow{
		ID: 3, UserID: mke, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 50,
		UpdateData: newMessageJSON(mke, erel, peerUser, 1, 50, "old-mke", true),
	}, PtsRow{
		ID: 4, UserID: mke, Pts: 80, PtsCount: 1, Date2: 50,
		UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":80}`,
	}, PtsRow{
		ID: 5, UserID: mke, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 300,
		UpdateData: newMessageJSON(erel, erel, peerUser, 1, 300, "hello", false),
	})
	snap.Messages = append(snap.Messages, Message{
		UserID: mke, BoxID: 40, SenderID: mke, PeerType: peerUser, PeerID: erel, Message: "kept", Date2: 40,
	})
	read := Counters{Box: map[int64]int64{erel: 2052, mke: 1400}, Pts: map[int64]int64{erel: 2153, mke: 1800}}
	plan := BuildPlan(snap, read)
	text := (Report{Plan: plan, Read: read, DryRun: true, Snap: snap}).Format("dry-run")
	t.Logf("\n%s", text)
	for _, want := range []string{
		"dry-run: rebuild 2 message rows",
		"counter message_box_ngen_1001=2052 db_max=1052 assign 2053-2053",
		"counter pts_updates_ngen_1001=2153 db_max=1153 assign 2154-2154",
		"counter message_box_ngen_1002=1400 db_max=40 assign 1401-1401",
		"dialog user=1001 peer_type=2 peer=1002 top=2053 read_inbox=1052 read_outbox=1052 unread 0 -> 0",
		"dialog user=1002 peer_type=2 peer=1001 top=1401 read_inbox=40 read_outbox=0 unread 17 -> 18",
		"dry-run only; -apply reserves these ids in Redis",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run missing %q\n%s", want, text)
		}
	}
}

func swallowedOne(erel int64) Snapshot {
	const mke = int64(1002)
	return Snapshot{
		Messages: []Message{
			{UserID: erel, BoxID: 1052, SenderID: erel, PeerType: peerUser, PeerID: mke, Message: "later", Date2: 250},
		},
		Pts: []PtsRow{
			{ID: 1, UserID: erel, Pts: 1153, PtsCount: 1, Date2: 250, UpdateData: `{"predicate_name":"updateReadHistoryInbox","pts_INT32":1153}`},
			{ID: 2, UserID: erel, Pts: 1, PtsCount: 1, UpdateType: ptsNewMessage, Date2: 300, UpdateData: newMessageJSON(erel, mke, peerUser, 1, 300, "hello", true)},
		},
	}
}
