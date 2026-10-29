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

// Command seqrepair rebuilds messages dropped when Redis id counters restarted
// at 1, and moves the duplicate pts rows above the current maximum.
//
// Dry-run (default) changes nothing:
//
//	go run ./teamgramd/deploy/seqrepair/cmd/seqrepair -dsn 'user:pass@tcp(127.0.0.1:3306)/teamgram?parseTime=true'
//
// Apply snapshots messages, user_pts_updates, and dialogs into
// *_bak_20261030_idreset when those tables are empty, then writes.
// Running it again does not replace the snapshot and does not insert twice.
//
//	go run ./teamgramd/deploy/seqrepair/cmd/seqrepair -dsn '...' -apply
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	"github.com/teamgram/teamgram-server/teamgramd/deploy/seqrepair"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("TEAMGRAM_MYSQL_DSN"), "MySQL DSN")
	apply := flag.Bool("apply", false, "write the repair; without this flag the command only prints the plan")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "missing -dsn or TEAMGRAM_MYSQL_DSN")
		os.Exit(2)
	}
	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	rep, err := seqrepair.Run(context.Background(), db, *apply)
	if rep.Plan.Rebuilt == 0 && rep.Plan.PtsMoved == 0 {
		fmt.Println("nothing to repair")
	} else {
		mode := "dry-run"
		if *apply {
			mode = "applied"
		}
		fmt.Printf("%s: rebuild %d message rows, renumber %d pts rows, update %d dialogs\n",
			mode, rep.Plan.Rebuilt, rep.Plan.PtsMoved, len(rep.Plan.Dialogs))
		for _, m := range rep.Plan.Inserts {
			fmt.Printf("  message user=%d box=%d peer_type=%d peer=%d date=%d %q\n",
				m.UserID, m.BoxID, m.PeerType, m.PeerID, m.Date2, clip(m.Message))
		}
		for _, d := range rep.Plan.Dialogs {
			fmt.Printf("  dialog user=%d peer_type=%d peer=%d top=%d read_inbox=%d read_outbox=%d unread=%d\n",
				d.UserID, d.PeerType, d.PeerID, d.TopMessage, d.ReadInbox, d.ReadOutbox, d.UnreadCount)
		}
	}
	if *apply {
		for _, name := range rep.Backups {
			fmt.Printf("backup %s\n", name)
		}
		fmt.Printf("duplicate (user_id, pts) pairs: %d\n", rep.DupPairs)
	} else if rep.Plan.Rebuilt != 0 || rep.Plan.PtsMoved != 0 {
		fmt.Println("dry-run only; pass -apply to snapshot the tables and write")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func clip(s string) string {
	if len(s) > 60 {
		return s[:60]
	}
	return s
}
