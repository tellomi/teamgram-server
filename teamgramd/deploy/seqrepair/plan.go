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

// Package seqrepair rebuilds private and group messages that were dropped
// when a flushed Redis id counter reused box ids 1..n. The old rows won the
// (user_id, user_message_box_id) unique key, and the new sends survived only
// in user_pts_updates.update_data.
package seqrepair

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
)

const mathMaxInt32 = math.MaxInt32

const (
	peerUser = 2
	peerChat = 3

	updateNewMessage            = "updateNewMessage"
	updateEditMessage           = "updateEditMessage"
	updateDeleteMessages        = "updateDeleteMessages"
	updateDeleteChannelMessages = "updateDeleteChannelMessages"

	ptsNewMessage            = 1
	ptsDeleteMessages        = 2
	ptsEditMessage           = 7
	ptsDeleteChannelMessages = 10
	ptsEditChannelMessage    = 11
)

// Message is one messages-table row.
type Message struct {
	UserID          int64
	BoxID           int32
	DialogID1       int64
	DialogID2       int64
	DialogMessageID int64
	SenderID        int64
	PeerType        int32
	PeerID          int64
	Message         string
	MessageData     string
	Date2           int64
	Deleted         bool
}

// PtsRow is one user_pts_updates row.
type PtsRow struct {
	ID         int64
	UserID     int64
	Pts        int32
	PtsCount   int32
	UpdateType int32
	UpdateData string
	Date2      int64
}

// Dialog is the slice of dialogs this repair rewrites.
type Dialog struct {
	UserID      int64
	PeerType    int32
	PeerID      int64
	TopMessage  int32
	ReadInbox   int32
	ReadOutbox  int32
	UnreadCount int32
	// UnreadBefore is the badge stored on the dialog row before this plan.
	// UnreadCount is recomputed from the messages that will exist after the
	// repair: incoming rows whose box id is above the read cursor. The
	// incident-day badges (17, 25, 6) are not written back.
	UnreadBefore int32
	Date2        int64
}

// Snapshot is the database state the planner reads.
type Snapshot struct {
	Messages []Message
	Pts      []PtsRow
	Dialogs  []Dialog
}

// PtsRewrite renumbers one post-reset pts row.
type PtsRewrite struct {
	ID         int64
	UserID     int64
	OldPts     int32
	NewPts     int32
	UpdateData string
}

// Plan is idempotent: applying it twice is the same as applying it once,
// because a second BuildPlan sees monotonic pts and the rebuilt rows.
type Plan struct {
	Inserts  []Message
	Pts      []PtsRewrite
	Dialogs  []Dialog
	Rebuilt  int
	PtsMoved int
	// BoxTo and PtsTo are the last id assigned for each user. Redis must be
	// at least this value before any later send.
	BoxTo map[int64]int64
	PtsTo map[int64]int64
}

// Counters are the live Redis values (the last id already issued).
// A zero value means the key was missing; the planner then uses the table max.
type Counters struct {
	Box map[int64]int64
	Pts map[int64]int64
}

// BuildPlan decides which swallowed messages to insert and which duplicate
// pts rows to move above max(database max, live counter).
//
// Only updateNewMessage rows are restored. updateEditMessage (and any other
// non-new pts type) is an edit of a message that already exists, and
// updateDeleteMessages means that message is gone. A second call sees the
// renumbered pts as already repaired and returns an empty plan.
func BuildPlan(s Snapshot, counters Counters) Plan {
	post := postResetPts(s.Pts)
	deleted := deletedBoxes(s.Pts)
	maxBox := map[int64]int32{}
	byBox := map[int64]map[int32]Message{}
	for _, m := range s.Messages {
		if m.BoxID > maxBox[m.UserID] {
			maxBox[m.UserID] = m.BoxID
		}
		if byBox[m.UserID] == nil {
			byBox[m.UserID] = map[int32]Message{}
		}
		byBox[m.UserID][m.BoxID] = m
	}
	for user, cur := range counters.Box {
		if cur > int64(maxBox[user]) && cur <= int64(mathMaxInt32) {
			maxBox[user] = int32(cur)
		}
	}

	type candidate struct {
		row    PtsRow
		parsed parsedMessage
		data   string
	}
	var candidates []candidate
	ptsNew := map[int64]int32{}
	ptsJSONBox := map[int64]int32{}

	for _, row := range post {
		parsed, ok := parseUpdate(row.UpdateData)
		if !ok {
			continue
		}
		parsed.finishOwner(row.UserID)
		if isNewMessage(row, parsed) {
			candidates = append(candidates, candidate{row: row, parsed: parsed, data: row.UpdateData})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return lessRow(candidates[i].row, candidates[j].row)
	})

	// dialog_message_id is shared by every box of one logical send.
	groupNth := map[string]int{}
	groupID := map[string]int64{}

	var inserts []Message
	rebuiltBox := map[int64]int32{} // pts row id -> new box id

	for _, c := range candidates {
		p := c.parsed
		if _, gone := deleted[p.Owner][p.BoxID]; gone {
			// A delete update names the box id. The message was removed, not lost.
			continue
		}
		stored, exists := byBox[p.Owner][p.BoxID]
		// Same sender and date is the same send. An edit changes the text in
		// place; that mismatch is not a swallowed duplicate-key insert.
		if exists && stored.Date2 == p.Date2 && stored.SenderID == p.Sender {
			continue
		}
		swallowed := !exists || stored.Date2 != p.Date2 || stored.SenderID != p.Sender
		if !swallowed {
			continue
		}
		if found, ok := findRestored(s.Messages, inserts, p); ok {
			rebuiltBox[c.row.ID] = found
			ptsJSONBox[c.row.ID] = found
			continue
		}
		key := convoKey(p)
		nth := groupNth[key]
		if _, ok := groupID[key]; !ok {
			groupID[key] = stableDialogMessageID(key, nth)
		}
		// A repeated identical text at the same second gets its own id only
		// when this owner already used the current one.
		if _, used := ownerUsedGroup(inserts, p.Owner, groupID[key]); used {
			nth++
			groupNth[key] = nth
			groupID[key] = stableDialogMessageID(key, nth)
		}
		maxBox[p.Owner]++
		box := maxBox[p.Owner]
		d1, d2 := dialogIDs(p.Owner, p.PeerType, p.PeerID)
		data := rewriteMessageID(p.raw, box)
		msg := Message{
			UserID:          p.Owner,
			BoxID:           box,
			DialogID1:       d1,
			DialogID2:       d2,
			DialogMessageID: groupID[key],
			SenderID:        p.Sender,
			PeerType:        p.PeerType,
			PeerID:          p.PeerID,
			Message:         p.Text,
			MessageData:     data,
			Date2:           p.Date2,
		}
		inserts = append(inserts, msg)
		rebuiltBox[c.row.ID] = box
		ptsJSONBox[c.row.ID] = box
		if byBox[p.Owner] == nil {
			byBox[p.Owner] = map[int32]Message{}
		}
		byBox[p.Owner][box] = msg
	}

	maxPts := map[int64]int32{}
	for _, row := range s.Pts {
		if row.Pts > maxPts[row.UserID] {
			maxPts[row.UserID] = row.Pts
		}
	}
	for user, cur := range counters.Pts {
		if cur > int64(maxPts[user]) && cur <= int64(mathMaxInt32) {
			maxPts[user] = int32(cur)
		}
	}
	var ptsRewrites []PtsRewrite
	ordered := append([]PtsRow(nil), post...)
	// Id order, not send time. New pts then increase with id, so a later pass
	// can tell these lifted rows from the continuation that still sits under them.
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].UserID != ordered[j].UserID {
			return ordered[i].UserID < ordered[j].UserID
		}
		return ordered[i].ID < ordered[j].ID
	})
	for _, row := range ordered {
		count := row.PtsCount
		if count <= 0 {
			count = 1
		}
		maxPts[row.UserID] += count
		newPts := maxPts[row.UserID]
		box, setBox := ptsJSONBox[row.ID]
		data := rewritePts(row.UpdateData, newPts, box, setBox)
		ptsRewrites = append(ptsRewrites, PtsRewrite{
			ID:         row.ID,
			UserID:     row.UserID,
			OldPts:     row.Pts,
			NewPts:     newPts,
			UpdateData: data,
		})
		ptsNew[row.ID] = newPts
	}

	dialogs := fixDialogs(s, inserts)
	boxTo := map[int64]int64{}
	for _, m := range inserts {
		if int64(m.BoxID) > boxTo[m.UserID] {
			boxTo[m.UserID] = int64(m.BoxID)
		}
	}
	ptsTo := map[int64]int64{}
	for _, rw := range ptsRewrites {
		if int64(rw.NewPts) > ptsTo[rw.UserID] {
			ptsTo[rw.UserID] = int64(rw.NewPts)
		}
	}
	return Plan{
		Inserts:  inserts,
		Pts:      ptsRewrites,
		Dialogs:  dialogs,
		Rebuilt:  len(inserts),
		PtsMoved: len(ptsRewrites),
		BoxTo:    boxTo,
		PtsTo:    ptsTo,
	}
}

// Apply returns the snapshot after the plan, used by tests and the dry-run diff.
func (p Plan) Apply(s Snapshot) Snapshot {
	out := Snapshot{
		Messages: append([]Message(nil), s.Messages...),
		Pts:      append([]PtsRow(nil), s.Pts...),
		Dialogs:  append([]Dialog(nil), s.Dialogs...),
	}
	out.Messages = append(out.Messages, p.Inserts...)
	byID := map[int64]PtsRewrite{}
	for _, rw := range p.Pts {
		byID[rw.ID] = rw
	}
	for i := range out.Pts {
		rw, ok := byID[out.Pts[i].ID]
		if !ok {
			continue
		}
		out.Pts[i].Pts = rw.NewPts
		out.Pts[i].UpdateData = rw.UpdateData
	}
	byDialog := map[dialogKey]Dialog{}
	for _, d := range p.Dialogs {
		byDialog[dialogKey{d.UserID, d.PeerType, d.PeerID}] = d
	}
	for i := range out.Dialogs {
		k := dialogKey{out.Dialogs[i].UserID, out.Dialogs[i].PeerType, out.Dialogs[i].PeerID}
		if d, ok := byDialog[k]; ok {
			out.Dialogs[i] = d
		}
	}
	return out
}

// DuplicatePts is the acceptance query:
// select user_id, pts, count(*) c from user_pts_updates group by 1, 2 having c>1
func DuplicatePts(rows []PtsRow) [][2]int64 {
	type key struct {
		user int64
		pts  int32
	}
	n := map[key]int{}
	for _, row := range rows {
		n[key{row.UserID, row.Pts}]++
	}
	var dups [][2]int64
	for k, c := range n {
		if c > 1 {
			dups = append(dups, [2]int64{k.user, int64(k.pts)})
		}
	}
	sort.Slice(dups, func(i, j int) bool {
		if dups[i][0] != dups[j][0] {
			return dups[i][0] < dups[j][0]
		}
		return dups[i][1] < dups[j][1]
	})
	return dups
}

type dialogKey struct {
	user     int64
	peerType int32
	peer     int64
}

func postResetPts(rows []PtsRow) []PtsRow {
	byUser := map[int64][]PtsRow{}
	for _, row := range rows {
		byUser[row.UserID] = append(byUser[row.UserID], row)
	}
	var out []PtsRow
	for _, list := range byUser {
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		out = append(out, resetRows(list)...)
	}
	return out
}

// resetRows is the post-flush segment for one user, in id order.
//
// A Redis restart writes a later row with a pts that already exists or that
// falls behind the high water. Those rows are moved above the current max.
// Doing that leaves them sitting above older rows that were inserted after
// the restart (higher ids, lower pts). A later pass must treat that shape as
// already repaired: the lifted rows stay, and the continuation is not moved
// again. A real restart still begins with a duplicate or a drop to a value
// that does not resume the pre-spike high water.
func resetRows(list []PtsRow) []PtsRow {
	seen := map[int32]bool{}
	var running int32
	var anchor int32
	var spikeHeight int32
	inSpike := false
	inCluster := false
	var out []PtsRow
	for _, row := range list {
		count := row.PtsCount
		if count <= 0 {
			count = 1
		}
		if seen[row.Pts] || (inCluster && row.Pts <= spikeHeight) {
			if !inCluster {
				spikeHeight = running
			}
			inCluster = true
			seen[row.Pts] = true
			out = append(out, row)
			continue
		}
		if inCluster && row.Pts > spikeHeight {
			inCluster = false
		}
		if running > 0 && row.Pts < running {
			// Continuation under rows that were already lifted above the max.
			// The value resumes the sequence from before that spike.
			if inSpike && anchor > 0 && row.Pts > anchor {
				inSpike = false
				seen[row.Pts] = true
				running = row.Pts
				anchor = row.Pts
				continue
			}
			spikeHeight = running
			inCluster = true
			seen[row.Pts] = true
			out = append(out, row)
			continue
		}
		if running > 0 && row.Pts > running+count {
			if !inSpike {
				anchor = running
				inSpike = true
			}
		} else if !inSpike {
			anchor = row.Pts
		}
		if row.Pts > running {
			running = row.Pts
		}
		seen[row.Pts] = true
	}
	return out
}

func lessRow(a, b PtsRow) bool {
	if a.UserID != b.UserID {
		return a.UserID < b.UserID
	}
	if a.Date2 != b.Date2 {
		return a.Date2 < b.Date2
	}
	return a.ID < b.ID
}

type parsedMessage struct {
	Predicate string
	Owner     int64
	BoxID     int32
	Sender    int64
	PeerType  int32
	PeerID    int64
	Text      string
	Date2     int64
	Out       bool
	raw       map[string]any
}

func isNewMessage(row PtsRow, p parsedMessage) bool {
	// update_type 1 is not enough: an edit can be stored with that column
	// while predicate_name is updateEditMessage. The predicate is the update
	// the client applied. A known non-new type (edit, delete, read) is never
	// a lost send, even if the JSON says updateNewMessage.
	switch p.Predicate {
	case updateEditMessage, updateDeleteMessages, updateDeleteChannelMessages:
		return false
	}
	if p.Predicate != updateNewMessage {
		return false
	}
	switch row.UpdateType {
	case ptsEditMessage, ptsEditChannelMessage, ptsDeleteMessages, ptsDeleteChannelMessages:
		return false
	case 0, ptsNewMessage:
		return p.BoxID != 0 || p.Text != "" || p.Date2 != 0
	default:
		return false
	}
}

func isDeleteUpdate(row PtsRow) bool {
	switch row.UpdateType {
	case ptsDeleteMessages, ptsDeleteChannelMessages:
		return true
	}
	switch predicateName(row.UpdateData) {
	case updateDeleteMessages, updateDeleteChannelMessages:
		return true
	}
	return false
}

func deletedBoxes(rows []PtsRow) map[int64]map[int32]struct{} {
	out := map[int64]map[int32]struct{}{}
	for _, row := range rows {
		if !isDeleteUpdate(row) {
			continue
		}
		for _, id := range deletedMessageIDs(row.UpdateData) {
			if id == 0 {
				continue
			}
			if out[row.UserID] == nil {
				out[row.UserID] = map[int32]struct{}{}
			}
			out[row.UserID][id] = struct{}{}
		}
	}
	return out
}

func predicateName(data string) string {
	var doc map[string]any
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return ""
	}
	name, _ := doc["predicate_name"].(string)
	return name
}

func deletedMessageIDs(data string) []int32 {
	var doc map[string]any
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return nil
	}
	var ids []int32
	if arr, ok := doc["messages"].([]any); ok {
		for _, v := range arr {
			if n := num(v); n != 0 {
				ids = append(ids, int32(n))
			}
		}
	}
	if n := num(doc["id_INT32"]); n != 0 {
		ids = append(ids, int32(n))
	}
	return ids
}

func parseUpdate(data string) (parsedMessage, bool) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return parsedMessage{}, false
	}
	pred, _ := doc["predicate_name"].(string)
	msg, _ := doc["message_MESSAGE"].(map[string]any)
	if pred != updateNewMessage && msg == nil {
		return parsedMessage{}, false
	}
	if msg == nil {
		return parsedMessage{}, false
	}
	p := parsedMessage{raw: msg, Predicate: pred}
	p.BoxID = int32(num(msg["id"]))
	p.Date2 = num(msg["date"])
	p.Text, _ = msg["message"].(string)
	p.Out, _ = msg["out"].(bool)
	from, _ := msg["from_id"].(map[string]any)
	peer, _ := msg["peer_id"].(map[string]any)
	if from != nil {
		p.Sender = num(from["user_id"])
	}
	p.PeerType, p.PeerID = peerOf(peer)
	return p, true
}

func peerOf(peer map[string]any) (int32, int64) {
	if peer == nil {
		return 0, 0
	}
	if id := num(peer["chat_id"]); id != 0 {
		return peerChat, id
	}
	if id := num(peer["channel_id"]); id != 0 {
		return 4, id
	}
	if id := num(peer["user_id"]); id != 0 {
		return peerUser, id
	}
	return 0, 0
}

func num(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

func (p *parsedMessage) finishOwner(owner int64) {
	p.Owner = owner
	if p.Sender != 0 {
		return
	}
	// Private inbox copies clear from_id and set peer_id to the sender
	// (sendMessageToInbox). out stays false. An outgoing copy has no from_id
	// either, and the sender is the owner.
	if !p.Out && (p.PeerType == peerUser || p.PeerType == 1) && p.PeerID != 0 {
		p.Sender = p.PeerID
		return
	}
	p.Sender = owner
}

func convoKey(p parsedMessage) string {
	switch p.PeerType {
	case peerUser, 1:
		a, b := p.Sender, p.PeerID
		if p.Owner != p.Sender {
			a, b = p.Sender, p.Owner
		}
		if a > b {
			a, b = b, a
		}
		return fmt.Sprintf("u:%d:%d:%d:%s", a, b, p.Date2, p.Text)
	default:
		return fmt.Sprintf("c:%d:%d:%d:%d:%s", p.PeerType, p.PeerID, p.Sender, p.Date2, p.Text)
	}
}

func stableDialogMessageID(key string, nth int) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(nth)))
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

func ownerUsedGroup(inserts []Message, owner, dialogMessageID int64) (int32, bool) {
	for _, m := range inserts {
		if m.UserID == owner && m.DialogMessageID == dialogMessageID {
			return m.BoxID, true
		}
	}
	return 0, false
}

func findRestored(existing, inserts []Message, p parsedMessage) (int32, bool) {
	for _, list := range [][]Message{inserts, existing} {
		for _, m := range list {
			if m.Deleted || m.UserID != p.Owner || m.BoxID == p.BoxID {
				continue
			}
			if m.SenderID == p.Sender && m.PeerType == p.PeerType && m.PeerID == p.PeerID && m.Date2 == p.Date2 && m.Message == p.Text {
				return m.BoxID, true
			}
		}
	}
	return 0, false
}

func dialogIDs(owner int64, peerType int32, peer int64) (int64, int64) {
	switch peerType {
	case peerUser, 1:
		a, b := owner, peer
		if a > b {
			a, b = b, a
		}
		return a, b
	default:
		return -int64(peerType), peer
	}
}

func rewriteMessageID(raw map[string]any, box int32) string {
	if raw == nil {
		b, _ := json.Marshal(map[string]any{"id": box})
		return string(b)
	}
	cp := cloneMap(raw)
	cp["id"] = box
	b, err := json.Marshal(cp)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func rewritePts(data string, pts int32, box int32, setBox bool) string {
	var doc map[string]any
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return data
	}
	doc["pts_INT32"] = pts
	if setBox {
		if msg, ok := doc["message_MESSAGE"].(map[string]any); ok {
			msg["id"] = box
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return data
	}
	return string(b)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func fixDialogs(s Snapshot, inserts []Message) []Dialog {
	type agg struct {
		msgs []Message
	}
	all := map[dialogKey]*agg{}
	add := func(m Message) {
		if m.Deleted {
			return
		}
		k := dialogKey{m.UserID, m.PeerType, m.PeerID}
		if all[k] == nil {
			all[k] = &agg{}
		}
		all[k].msgs = append(all[k].msgs, m)
	}
	inserted := map[dialogKey][]Message{}
	for _, m := range inserts {
		add(m)
		k := dialogKey{m.UserID, m.PeerType, m.PeerID}
		inserted[k] = append(inserted[k], m)
	}
	for _, m := range s.Messages {
		add(m)
	}

	var out []Dialog
	for _, d := range s.Dialogs {
		k := dialogKey{d.UserID, d.PeerType, d.PeerID}
		bucket := all[k]
		if bucket == nil || len(inserted[k]) == 0 {
			continue
		}
		var top int32
		var topDate int64
		var oldMax int32
		newIDs := map[int32]bool{}
		var newOutgoing []int32
		for _, m := range inserted[k] {
			newIDs[m.BoxID] = true
			if m.SenderID == d.UserID {
				newOutgoing = append(newOutgoing, m.BoxID)
			}
		}
		for _, m := range bucket.msgs {
			if m.BoxID > top {
				top = m.BoxID
				topDate = m.Date2
			}
			if !newIDs[m.BoxID] && m.BoxID > oldMax {
				oldMax = m.BoxID
			}
		}
		next := d
		next.UnreadBefore = d.UnreadCount
		next.TopMessage = top
		if topDate != 0 {
			next.Date2 = topDate
		}
		next.ReadInbox = clampRead(d.ReadInbox, oldMax, keys(newIDs))
		next.ReadOutbox = clampRead(d.ReadOutbox, oldMax, newOutgoing)
		// The badge is the incoming rows above the cursor after the repair.
		// Restored sends get box ids above max(table, Redis), so they sit at
		// the top of the chat and count as unread. Their date2 stays the
		// original send time. The stored badge is not an input: it can be
		// stale (the private chat between the two affected users is 2 now,
		// not the 6 recorded on the day of the flush).
		next.UnreadCount = unreadAbove(bucket.msgs, d.UserID, next.ReadInbox)
		out = append(out, next)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UserID != out[j].UserID {
			return out[i].UserID < out[j].UserID
		}
		if out[i].PeerType != out[j].PeerType {
			return out[i].PeerType < out[j].PeerType
		}
		return out[i].PeerID < out[j].PeerID
	})
	return out
}

// unreadAbove counts incoming messages the client will treat as unread:
// box id above the read cursor, sender is not the dialog owner.
func unreadAbove(msgs []Message, owner int64, readInbox int32) int32 {
	var n int32
	for _, m := range msgs {
		if m.Deleted || m.SenderID == owner || m.BoxID <= readInbox {
			continue
		}
		n++
	}
	return n
}

func keys(m map[int32]bool) []int32 {
	out := make([]int32, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

// clampRead leaves a watermark that is already below every rebuilt id.
// A watermark that reaches a rebuilt id is pulled back to the newest message
// that was already in the dialog, so the rebuilt sends stay unread.
func clampRead(current, oldMax int32, newIDs []int32) int32 {
	if len(newIDs) == 0 {
		return current
	}
	minNew := newIDs[0]
	for _, id := range newIDs[1:] {
		if id < minNew {
			minNew = id
		}
	}
	if current < minNew {
		return current
	}
	if oldMax < minNew {
		return oldMax
	}
	return minNew - 1
}
