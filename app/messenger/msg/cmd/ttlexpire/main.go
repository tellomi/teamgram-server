// Command ttlexpire rewrites ttl media that was already opened and whose
// countdown has elapsed. It does not touch unopened media.
//
//	ttlexpire -f /app/etc/msg.yaml
//	ttlexpire -f /app/etc/msg.yaml -dry-run=false -backup /var/backups/ttl-media.json
//
// The default is a dry run. A live run refuses to start without -backup.
// The backup is the message_data of every row that will change, written and
// synced before the first update. It is not printed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/config"
	"github.com/teamgram/teamgram-server/app/service/ttlmedia"
	"github.com/teamgram/teamgram-server/pkg/configload"
)

func main() {
	file := flag.String("f", "etc/msg.yaml", "msg config file, for the MySQL DSN")
	dry := flag.Bool("dry-run", true, "report what would expire and change nothing")
	backup := flag.String("backup", "", "file to write before any update; required when -dry-run=false")
	flag.Parse()

	var cfg config.Config
	configload.MustLoad(*file, &cfg)
	st, err := ttlmedia.Open(cfg.Mysql.DSN)
	if err != nil {
		log.Fatal(err)
	}
	if st == nil {
		log.Fatal("mysql dsn is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	now := int32(time.Now().Unix())
	dialogs, err := st.Scan(ctx, now)
	if err != nil {
		log.Fatal(err)
	}
	var expire, unopened, noSignal, notDue int
	var hits []ttlmedia.Dialog
	for _, d := range dialogs {
		switch d.Class {
		case ttlmedia.ClassExpire:
			expire++
			hits = append(hits, d)
		case ttlmedia.ClassSkipUnopened:
			unopened++
		case ttlmedia.ClassSkipNoSignal:
			noSignal++
		default:
			notDue++
		}
	}
	mode := "DRY_RUN"
	if !*dry {
		mode = "LIVE"
	}
	fmt.Printf("ttl media %s: expire=%d skip_unopened=%d skip_no_signal=%d skip_not_due=%d\n",
		mode, expire, unopened, noSignal, notDue)
	for _, d := range hits {
		fmt.Printf("  dialog=%d ttl=%d kind=%s media=%d copies=%d\n", d.DialogID, d.TTL, d.Kind, d.MediaID, len(d.Copies))
	}
	if *dry {
		return
	}
	if *backup == "" {
		log.Fatal("-backup is required when -dry-run=false")
	}
	if err = writeBackup(*backup, hits); err != nil {
		log.Fatalf("backup: %v", err)
	}
	applied := 0
	for _, d := range hits {
		if err = st.Remember(ctx, d, now); err != nil {
			log.Fatalf("remember dialog %d: %v", d.DialogID, err)
		}
		if err = st.Strip(ctx, d.DialogID); err != nil {
			log.Fatalf("strip dialog %d after %d applied: %v", d.DialogID, applied, err)
		}
		applied++
	}
	fmt.Printf("ttl media LIVE: applied=%d backup=%s\n", applied, *backup)
}

func writeBackup(path string, hits []ttlmedia.Dialog) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	type row struct {
		Table       string `json:"table"`
		UserID      int64  `json:"user_id"`
		BoxID       int32  `json:"user_message_box_id"`
		DialogID    int64  `json:"dialog_message_id"`
		MediaUnread bool   `json:"media_unread"`
		MessageData string `json:"message_data"`
	}
	var rows []row
	for _, d := range hits {
		for _, c := range d.Copies {
			rows = append(rows, row{
				Table: c.Table, UserID: c.UserID, BoxID: c.BoxID, DialogID: c.DialogID,
				MediaUnread: c.MediaUnread, MessageData: c.MessageData,
			})
		}
	}
	if rows == nil {
		rows = []row{}
	}
	if err = enc.Encode(rows); err != nil {
		return err
	}
	return f.Sync()
}
