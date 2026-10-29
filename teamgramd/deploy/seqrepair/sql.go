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
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const backupSuffix = "_bak_20261030_idreset"

// Report is what a dry-run or apply prints.
type Report struct {
	Plan     Plan
	Read     Counters
	DryRun   bool
	Backups  []string
	DupPairs int
	Snap     Snapshot
}

// Run loads MySQL, builds the plan, and either prints it or writes it.
// Apply snapshots messages, user_pts_updates, and dialogs first, then reserves
// box and pts ids in Redis above the live counter and writes those ids.
// A second run keeps the snapshot and does not insert or move the counter again.
func Run(ctx context.Context, db *sql.DB, seq Sequencer, apply bool) (Report, error) {
	snap, err := Load(ctx, db)
	if err != nil {
		return Report{}, err
	}
	read, err := readCounters(ctx, seq, snap)
	if err != nil {
		return Report{}, err
	}
	plan := BuildPlan(snap, read)
	rep := Report{Plan: plan, Read: read, DryRun: !apply, Snap: snap}
	if !apply {
		return rep, nil
	}
	if seq == nil && (len(plan.Inserts) > 0 || len(plan.Pts) > 0) {
		return rep, fmt.Errorf("refusing to apply without Redis: new ids would sit under the live counter and the next send could reuse them")
	}
	backed, err := backupAll(ctx, db)
	if err != nil {
		return rep, err
	}
	rep.Backups = backed
	if seq != nil {
		plan, err = reserveAndPlan(ctx, seq, snap, read, plan)
		if err != nil {
			return rep, err
		}
		rep.Plan = plan
	}
	if err = writePlan(ctx, db, plan); err != nil {
		return rep, err
	}
	n, err := countDuplicatePts(ctx, db)
	if err != nil {
		return rep, err
	}
	rep.DupPairs = n
	if n != 0 {
		return rep, fmt.Errorf("acceptance failed: %d duplicate (user_id, pts) pairs remain", n)
	}
	return rep, nil
}

func readCounters(ctx context.Context, seq Sequencer, snap Snapshot) (Counters, error) {
	out := Counters{Box: map[int64]int64{}, Pts: map[int64]int64{}}
	if seq == nil {
		return out, nil
	}
	users := map[int64]struct{}{}
	for _, m := range snap.Messages {
		users[m.UserID] = struct{}{}
	}
	for _, row := range snap.Pts {
		users[row.UserID] = struct{}{}
	}
	for user := range users {
		box, err := seq.Get(ctx, boxKey(user))
		if err != nil {
			return out, fmt.Errorf("get %s: %w", boxKey(user), err)
		}
		pts, err := seq.Get(ctx, ptsKey(user))
		if err != nil {
			return out, fmt.Errorf("get %s: %w", ptsKey(user), err)
		}
		out.Box[user] = parseCounter(box)
		out.Pts[user] = parseCounter(pts)
	}
	return out, nil
}

func parseCounter(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// reserveAndPlan claims the id ranges in Redis, then rebuilds the plan from
// the floors those claims started at. A send between the dry-run and apply
// moves the claim upward instead of colliding with it.
func reserveAndPlan(ctx context.Context, seq Sequencer, snap Snapshot, read Counters, preview Plan) (Plan, error) {
	fresh := Counters{Box: map[int64]int64{}, Pts: map[int64]int64{}}
	for user, n := range boxClaim(snap, read, preview) {
		first, _, err := seq.Reserve(ctx, boxKey(user), int64(maxBoxID(snap, user)), n)
		if err != nil {
			return Plan{}, fmt.Errorf("reserve box ids for %d: %w", user, err)
		}
		fresh.Box[user] = first - 1
	}
	for user, n := range ptsClaim(snap, read, preview) {
		first, _, err := seq.Reserve(ctx, ptsKey(user), int64(maxPtsID(snap, user)), n)
		if err != nil {
			return Plan{}, fmt.Errorf("reserve pts for %d: %w", user, err)
		}
		fresh.Pts[user] = first - 1
	}
	// Keep counters for users we did not reserve so a later id still clears them.
	for user, v := range read.Box {
		if _, ok := fresh.Box[user]; !ok {
			fresh.Box[user] = v
		}
	}
	for user, v := range read.Pts {
		if _, ok := fresh.Pts[user]; !ok {
			fresh.Pts[user] = v
		}
	}
	return BuildPlan(snap, fresh), nil
}

func boxClaim(snap Snapshot, read Counters, plan Plan) map[int64]int64 {
	db := map[int64]int64{}
	for _, m := range snap.Messages {
		if int64(m.BoxID) > db[m.UserID] {
			db[m.UserID] = int64(m.BoxID)
		}
	}
	out := map[int64]int64{}
	for user, to := range plan.BoxTo {
		base := db[user]
		if read.Box[user] > base {
			base = read.Box[user]
		}
		if to > base {
			out[user] = to - base
		}
	}
	return out
}

func ptsClaim(snap Snapshot, read Counters, plan Plan) map[int64]int64 {
	out := map[int64]int64{}
	for user, to := range plan.PtsTo {
		base := int64(maxPtsID(snap, user))
		if read.Pts[user] > base {
			base = read.Pts[user]
		}
		if to > base {
			out[user] = to - base
		}
	}
	return out
}

func maxBoxID(snap Snapshot, user int64) int32 {
	var max int32
	for _, m := range snap.Messages {
		if m.UserID == user && m.BoxID > max {
			max = m.BoxID
		}
	}
	return max
}

// Format is the dry-run / apply text.
func (r Report) Format(mode string) string {
	var b strings.Builder
	if r.Plan.Rebuilt == 0 && r.Plan.PtsMoved == 0 {
		b.WriteString("nothing to repair\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%s: rebuild %d message rows, renumber %d pts rows, update %d dialogs\n",
		mode, r.Plan.Rebuilt, r.Plan.PtsMoved, len(r.Plan.Dialogs))
	users := map[int64]struct{}{}
	for user := range r.Plan.BoxTo {
		users[user] = struct{}{}
	}
	for user := range r.Plan.PtsTo {
		users[user] = struct{}{}
	}
	ids := make([]int64, 0, len(users))
	for user := range users {
		ids = append(ids, user)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, user := range ids {
		if to, ok := r.Plan.BoxTo[user]; ok {
			fmt.Fprintf(&b, "counter %s=%d db_max=%d assign %d-%d\n",
				boxKey(user), r.Read.Box[user], maxBoxID(r.Snap, user), minBox(r.Plan, user), to)
		}
		if to, ok := r.Plan.PtsTo[user]; ok {
			fmt.Fprintf(&b, "counter %s=%d db_max=%d assign %d-%d\n",
				ptsKey(user), r.Read.Pts[user], maxPtsID(r.Snap, user), minPts(r.Plan, user), to)
		}
	}
	for _, m := range r.Plan.Inserts {
		fmt.Fprintf(&b, "  message user=%d box=%d sender=%d peer_type=%d peer=%d date=%d %q\n",
			m.UserID, m.BoxID, m.SenderID, m.PeerType, m.PeerID, m.Date2, clipText(m.Message))
	}
	for _, d := range r.Plan.Dialogs {
		fmt.Fprintf(&b, "  dialog user=%d peer_type=%d peer=%d top=%d read_inbox=%d read_outbox=%d unread %d -> %d\n",
			d.UserID, d.PeerType, d.PeerID, d.TopMessage, d.ReadInbox, d.ReadOutbox, d.UnreadBefore, d.UnreadCount)
	}
	if r.DryRun {
		b.WriteString("dry-run only; -apply reserves these ids in Redis and writes. A send before -apply moves the range up.\n")
	}
	return b.String()
}

func minBox(plan Plan, user int64) int32 {
	var min int32
	for _, m := range plan.Inserts {
		if m.UserID == user && (min == 0 || m.BoxID < min) {
			min = m.BoxID
		}
	}
	return min
}

func minPts(plan Plan, user int64) int32 {
	var min int32
	for _, rw := range plan.Pts {
		if rw.UserID == user && (min == 0 || rw.NewPts < min) {
			min = rw.NewPts
		}
	}
	return min
}

func clipText(s string) string {
	if len(s) > 60 {
		return s[:60]
	}
	return s
}

func maxPtsID(snap Snapshot, user int64) int32 {
	var max int32
	for _, row := range snap.Pts {
		if row.UserID == user && row.Pts > max {
			max = row.Pts
		}
	}
	return max
}

// Load reads the rows the planner needs.
func Load(ctx context.Context, db *sql.DB) (Snapshot, error) {
	var snap Snapshot
	tables, err := messageTables(ctx, db)
	if err != nil {
		return snap, err
	}
	for _, table := range tables {
		rows, err := db.QueryContext(ctx, `
SELECT user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id,
       sender_user_id, peer_type, peer_id, message, IFNULL(message_data, ''), date2, deleted
FROM `+quote(table))
		if err != nil {
			return snap, fmt.Errorf("%s: %w", table, err)
		}
		for rows.Next() {
			var m Message
			var deleted int
			if err = rows.Scan(&m.UserID, &m.BoxID, &m.DialogID1, &m.DialogID2, &m.DialogMessageID,
				&m.SenderID, &m.PeerType, &m.PeerID, &m.Message, &m.MessageData, &m.Date2, &deleted); err != nil {
				rows.Close()
				return snap, err
			}
			m.Deleted = deleted != 0
			snap.Messages = append(snap.Messages, m)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return snap, err
		}
		rows.Close()
	}

	pts, err := db.QueryContext(ctx, `
SELECT id, user_id, pts, pts_count, update_type, update_data, date2 FROM user_pts_updates`)
	if err != nil {
		return snap, err
	}
	defer pts.Close()
	for pts.Next() {
		var row PtsRow
		var data []byte
		if err = pts.Scan(&row.ID, &row.UserID, &row.Pts, &row.PtsCount, &row.UpdateType, &data, &row.Date2); err != nil {
			return snap, err
		}
		row.UpdateData = string(data)
		snap.Pts = append(snap.Pts, row)
	}
	if err = pts.Err(); err != nil {
		return snap, err
	}

	dialogs, err := db.QueryContext(ctx, `
SELECT user_id, peer_type, peer_id, top_message, read_inbox_max_id, read_outbox_max_id, unread_count, date2
FROM dialogs`)
	if err != nil {
		return snap, err
	}
	defer dialogs.Close()
	for dialogs.Next() {
		var d Dialog
		if err = dialogs.Scan(&d.UserID, &d.PeerType, &d.PeerID, &d.TopMessage, &d.ReadInbox, &d.ReadOutbox, &d.UnreadCount, &d.Date2); err != nil {
			return snap, err
		}
		snap.Dialogs = append(snap.Dialogs, d)
	}
	return snap, dialogs.Err()
}

func writePlan(ctx context.Context, db *sql.DB, plan Plan) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	tables, err := messageTables(ctx, db)
	if err != nil {
		return err
	}
	for _, m := range plan.Inserts {
		table := messageTableFor(tables, m.UserID)
		_, err = tx.ExecContext(ctx, `
INSERT INTO `+quote(table)+`
  (user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id,
   sender_user_id, peer_type, peer_id, message_data, message, date2, deleted)
VALUES (?,?,?,?,?,?,?,?,?,?,?,0)`,
			m.UserID, m.BoxID, m.DialogID1, m.DialogID2, m.DialogMessageID,
			m.SenderID, m.PeerType, m.PeerID, m.MessageData, m.Message, m.Date2)
		if err != nil {
			return fmt.Errorf("insert user %d box %d: %w", m.UserID, m.BoxID, err)
		}
	}
	for _, rw := range plan.Pts {
		res, err := tx.ExecContext(ctx, `
UPDATE user_pts_updates SET pts=?, update_data=? WHERE id=? AND pts=?`,
			rw.NewPts, rw.UpdateData, rw.ID, rw.OldPts)
		if err != nil {
			return fmt.Errorf("pts row %d: %w", rw.ID, err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			var cur int32
			err = tx.QueryRowContext(ctx, `SELECT pts FROM user_pts_updates WHERE id=?`, rw.ID).Scan(&cur)
			if err != nil {
				return fmt.Errorf("pts row %d missing after update: %w", rw.ID, err)
			}
			if cur != rw.NewPts {
				return fmt.Errorf("pts row %d not updated (current %d, wanted %d)", rw.ID, cur, rw.NewPts)
			}
		}
	}
	for _, d := range plan.Dialogs {
		_, err = tx.ExecContext(ctx, `
UPDATE dialogs
SET top_message=?, read_inbox_max_id=?, read_outbox_max_id=?, unread_count=?, date2=?
WHERE user_id=? AND peer_type=? AND peer_id=?`,
			d.TopMessage, d.ReadInbox, d.ReadOutbox, d.UnreadCount, d.Date2,
			d.UserID, d.PeerType, d.PeerID)
		if err != nil {
			return fmt.Errorf("dialog %d/%d/%d: %w", d.UserID, d.PeerType, d.PeerID, err)
		}
	}
	return tx.Commit()
}

func backupAll(ctx context.Context, db *sql.DB) ([]string, error) {
	tables, err := messageTables(ctx, db)
	if err != nil {
		return nil, err
	}
	tables = append(tables, "user_pts_updates", "dialogs")
	var backed []string
	for _, table := range tables {
		dst := table + backupSuffix
		if err = backupTable(ctx, db, table, dst); err != nil {
			return backed, err
		}
		backed = append(backed, dst)
	}
	return backed, nil
}

func backupTable(ctx context.Context, db *sql.DB, src, dst string) error {
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+quote(dst)+" LIKE "+quote(src)); err != nil {
		return fmt.Errorf("backup %s: %w", src, err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quote(dst)).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+quote(dst)+" SELECT * FROM "+quote(src)); err != nil {
		return fmt.Errorf("snapshot %s: %w", src, err)
	}
	return nil
}

func countDuplicatePts(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM (
  SELECT user_id, pts FROM user_pts_updates GROUP BY user_id, pts HAVING COUNT(*) > 1
) d`).Scan(&n)
	return n, err
}

func messageTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT table_name FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND (table_name = 'messages' OR table_name REGEXP '^messages_[0-9]+$')
ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no messages table in this database")
	}
	return tables, nil
}

// messageTableFor picks the shard that stores this user's box.
// An unsharded database has only "messages".
func messageTableFor(tables []string, userID int64) string {
	maxShard := -1
	hasBase := false
	for _, name := range tables {
		if name == "messages" {
			hasBase = true
			continue
		}
		var n int
		if _, err := fmt.Sscanf(name, "messages_%d", &n); err == nil && n > maxShard {
			maxShard = n
		}
	}
	if maxShard < 0 {
		if hasBase || len(tables) == 0 {
			return "messages"
		}
		return tables[0]
	}
	return fmt.Sprintf("messages_%d", userID%int64(maxShard+1))
}

func quote(name string) string {
	if strings.ContainsAny(name, "`;'\"") {
		panic("unexpected table name " + name)
	}
	return "`" + name + "`"
}
