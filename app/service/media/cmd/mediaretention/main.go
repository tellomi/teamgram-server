// Command mediaretention runs one media retention sweep and prints the
// report. It reads the same media.yaml (for the MySQL DSN) and
// TELLOMI_MEDIA_RETENTION_* / TELLOMI_S3_* environment as the media service.
//
//	mediaretention -f /app/etc2/media.yaml -dry-run
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/media/internal/server"
	"github.com/teamgram/teamgram-server/app/service/media/retention"
	"github.com/teamgram/teamgram-server/pkg/configload"
)

func main() {
	// -f (the media config file, for the MySQL DSN) is registered by the
	// media server package, which this command imports for NewSweeper.
	dryRun := flag.Bool("dry-run", true, "report what would expire without deleting anything")
	days := flag.Int("days", 0, "override TELLOMI_MEDIA_RETENTION_DAYS")
	flag.Parse()

	cfg, err := retention.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	cfg.DryRun = *dryRun
	if *days > 0 {
		cfg.Days = *days
	}
	var c config.Config
	configload.MustLoad(flag.Lookup("f").Value.String(), &c)
	sw, err := server.NewSweeper(c, cfg)
	if err != nil {
		log.Fatal(err)
	}
	sw.Logf = func(format string, args ...any) { log.Printf(format, args...) }
	log.Printf("media retention one-shot: dry_run=%v days=%d grace_days=%d batch=%d max_per_run=%d enabled_in_env=%v",
		cfg.DryRun, cfg.Days, cfg.GraceDays, cfg.Batch, cfg.MaxPerRun, cfg.Enabled)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	rep, err := sw.Run(ctx, time.Now())
	fmt.Println(rep.String())
	for _, s := range rep.Samples {
		fmt.Println("  " + s)
	}
	if err != nil {
		log.Printf("run failed: %v", err)
		os.Exit(1)
	}
}
