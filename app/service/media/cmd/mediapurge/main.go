// Command mediapurge backfills message_media_refs and reports how many
// objects a later live run would free. It does not delete those objects.
//
//	mediapurge -f /app/etc2/media.yaml
//	mediapurge -f /app/etc2/media.yaml -dry-run=false -grace=0
//
// -dry-run is true by default and never deletes. -dry-run=false processes
// the purge queue (ids enqueued by deletes). Historical objects are only
// listed. -grace=0 processes queued ids whose grace has not elapsed; the
// worker still re-checks that nothing live holds them.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/mediapurge"
	"github.com/teamgram/teamgram-server/pkg/configload"
)

func main() {
	configFile := flag.String("f", "/app/etc2/media.yaml", "media config, for the MySQL DSN")
	dryRun := flag.Bool("dry-run", true, "report only; do not delete objects")
	backfill := flag.Bool("backfill", true, "insert message_media_refs for live messages")
	grace := flag.String("grace", "", "empty processes only due ids; 0 processes every queued id")
	flag.Parse()

	var c config.Config
	configload.MustLoad(*configFile, &c)
	st, err := mediapurge.Open(c.Mysql.DSN)
	if err != nil {
		log.Fatal(err)
	}
	if st == nil {
		log.Fatal("mysql dsn is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	rep, err := st.DryRun(ctx, *backfill)
	fmt.Println(rep.String())
	for _, s := range rep.Samples {
		fmt.Println("  " + s)
	}
	if err != nil {
		log.Printf("report failed: %v", err)
		os.Exit(1)
	}
	if *dryRun {
		log.Printf("media purge one-shot: dry_run=true backfill=%v (no objects deleted)", *backfill)
		return
	}
	var del mediapurge.Deleter
	objs, err := objectstore.Open()
	if err != nil {
		log.Fatal(err)
	}
	if objs == nil {
		log.Fatal("object storage (TELLOMI_S3_*) is not configured")
	}
	del = objs
	result, err := st.Run(ctx, del, time.Now(), 500, false, *grace == "0" || *grace == "0s")
	fmt.Println(result.String())
	for _, s := range result.Samples {
		fmt.Println("  " + s)
	}
	if err != nil {
		log.Printf("queue run failed: %v", err)
		os.Exit(1)
	}
}
