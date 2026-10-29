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
	"sort"
	"strconv"
)

const (
	peerUser = 2
	peerChat = 3

	updateNewMessage = "updateNewMessage"
	ptsNewMessage    = 1
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
	Date2       int64
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
}

// BuildPlan decides which swallowed messages to insert and which duplicate
// pts rows to move above the current per-user maximum.
func BuildPlan(s Snapshot) Plan {
	post := postResetPts(s.Pts)
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
		stored, exists := byBox[p.Owner][p.BoxID]
		swallowed := !exists || stored.Date2 != p.Date2 || stored.Message != p.Text || stored.SenderID != p.Sender
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
	var ptsRewrites []PtsRewrite
	ordered := append([]PtsRow(nil), post...)
	sort.Slice(ordered, func(i, j int) bool { return lessRow(ordered[i], ordered[j]) })
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
	return Plan{
		Inserts:  inserts,
		Pts:      ptsRewrites,
		Dialogs:  dialogs,
		Rebuilt:  len(inserts),
		PtsMoved: len(ptsRewrites),
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
		first := map[int32]int64{}
		var maxPts int32
		for _, row := range list {
			if id, ok := first[row.Pts]; ok && id != row.ID {
				out = append(out, row)
				continue
			}
			if _, ok := first[row.Pts]; !ok && maxPts > 0 && row.Pts < maxPts {
				out = append(out, row)
				continue
			}
			first[row.Pts] = row.ID
			if row.Pts > maxPts {
				maxPts = row.Pts
			}
		}
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
	if p.Predicate != updateNewMessage && row.UpdateType != ptsNewMessage {
		return false
	}
	return p.BoxID != 0 || p.Text != "" || p.Date2 != 0
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
	if p.Sender == 0 {
		p.Sender = owner
	}
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
		next.TopMessage = top
		if topDate != 0 {
			next.Date2 = topDate
		}
		next.ReadInbox = clampRead(d.ReadInbox, oldMax, keys(newIDs))
		next.ReadOutbox = clampRead(d.ReadOutbox, oldMax, newOutgoing)
		var unread int32
		for _, m := range bucket.msgs {
			if m.BoxID > next.ReadInbox {
				unread++
			}
		}
		next.UnreadCount = unread
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
