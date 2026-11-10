// Command backfillstripped writes the inline profile-photo thumb (size "i")
// for photos that were stored before uploads started saving it.
//
// It only inserts a photo_sizes row (or fills one whose cached bytes are
// empty). It does not delete rows or objects. Run it by hand, in batches.
// Pass -after with the last id from the previous run to resume.
//
//	backfillstripped -f /app/etc2/dfs.yaml -dry-run
//	backfillstripped -f /app/etc2/dfs.yaml -dry-run=false -redis redis:6379
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/strippedbackfill"
	"github.com/teamgram/teamgram-server/pkg/configload"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func main() {
	configFile := flag.String("f", strippedbackfill.ContainerDFSConfig, "dfs yaml inside the teamgram container, for the MySQL DSN and object storage")
	dryRun := flag.Bool("dry-run", true, "read photos and report what would be stored, without writing")
	batch := flag.Int("batch", 50, "profile photos to load per query")
	after := flag.Int64("after", 0, "resume after this photo id (the last id printed by the previous run)")
	maxN := flag.Int("max", 0, "stop after this many photos; 0 walks every profile photo")
	redisAddr := flag.String("redis", "", "redis host:port. On the compose network this is redis:6379. Drops photo#, user_data.2#, and chat# for each stored thumb. Empty leaves those caches until they expire (about 7 days)")
	flag.Parse()

	var c config.Config
	configload.MustLoad(*configFile, &c)
	if err := c.Minio.ApplyEnv(); err != nil {
		log.Fatal(err)
	}
	if c.Mysql.DSN == "" {
		log.Fatal("mysql DSN is empty in the dfs config")
	}
	db, err := mysqltls.Open(c.Mysql.DSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(time.Hour)

	store := minio_util.MustNewMinioClient(&c.Minio)
	var cache strippedbackfill.Cache
	drop := *redisAddr != "" && !*dryRun
	if drop {
		cache = strippedbackfill.Redis{Client: redis.New(*redisAddr)}
	}

	log.Printf("stripped thumb backfill: dry_run=%v batch=%d after=%d max=%d drop_cache=%v",
		*dryRun, *batch, *after, *maxN, drop)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	rep, err := strippedbackfill.Run(ctx, strippedbackfill.NewMySQL(db), strippedbackfill.Objects{Minio: store}, cache, strippedbackfill.Options{
		After:     *after,
		Batch:     *batch,
		Max:       *maxN,
		DryRun:    *dryRun,
		DropCache: drop,
		Encode:    imaging.StrippedJPEG,
	})
	fmt.Println(rep.String())
	if rep.Last > 0 {
		fmt.Printf("resume with -after %d\n", rep.Last)
	}
	if *redisAddr == "" && !*dryRun && rep.Filled > 0 {
		log.Printf("caches were not dropped. Clients keep the old photo for about 7 days unless you re-run with -redis")
	}
	if err != nil {
		log.Printf("stopped: %v", err)
		os.Exit(1)
	}
}
