// Command backfillthumbs finds photo_sizes rows whose object is missing
// and rebuilds a JPEG from the source document when ffmpeg can read it.
// Rows that cannot be rebuilt are printed as FLAG lines. Nothing is deleted.
//
// Dry-run is the default. It does not write objects or file_size.
//
//	backfillthumbs -f /app/etc2/dfs.yaml -dry-run
//	backfillthumbs -f /app/etc2/dfs.yaml -dry-run=false
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/ffmpegutil"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/thumbbackfill"
	"github.com/teamgram/teamgram-server/pkg/configload"
)

func main() {
	configFile := flag.String("f", "/app/etc2/dfs.yaml", "dfs yaml inside the teamgram container, for MySQL and object storage")
	dryRun := flag.Bool("dry-run", true, "list missing thumbs without writing")
	batch := flag.Int("batch", 100, "photo_sizes rows to load per query")
	after := flag.Int64("after", 0, "resume after this photo_sizes.id")
	maxN := flag.Int("max", 0, "stop after this many rows; 0 walks every stored size")
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

	log.Printf("thumb backfill: dry_run=%v batch=%d after=%d max=%d", *dryRun, *batch, *after, *maxN)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	rep, err := thumbbackfill.Run(ctx, thumbbackfill.MySQL{DB: db}, thumbbackfill.Objects{Minio: minio_util.MustNewMinioClient(&c.Minio)}, thumbbackfill.Options{
		After:  *after,
		Batch:  *batch,
		Max:    *maxN,
		DryRun: *dryRun,
		Frame:  frameFunc(),
	})
	for _, line := range rep.Flags {
		fmt.Println(line)
	}
	fmt.Println(rep.String())
	if rep.Last > 0 {
		fmt.Printf("resume with -after %d\n", rep.Last)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func frameFunc() thumbbackfill.Frame {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("ffmpeg not found; missing thumbs will be flagged")
		return nil
	}
	ff := ffmpegutil.NewFFmpegUtil()
	return func(src []byte) ([]byte, error) {
		frame, err := ff.GetFirstFrameByPipe(src)
		if err != nil || len(frame) == 0 {
			if err == nil {
				err = errors.New("ffmpeg produced no frame")
			}
			return nil, err
		}
		return thumbbackfill.FitJPEG(frame)
	}
}
