// Command importgifs loads mp4 and gif clips into the GIF panel.
// It uploads bytes with the same DFS client as importstickers and upserts
// tellomi_gif_clips. Running it again updates the same rows. The bff
// reloads the catalog within a minute; it does not need a restart.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("TEAMGRAM_MYSQL_DSN"), "MySQL DSN; defaults to TEAMGRAM_MYSQL_DSN")
	dir := flag.String("dir", "", "directory of mp4/gif clips, with manifest.json or one file per clip")
	dry := flag.Bool("dry-run", false, "validate -dir and print the plan without writing")
	flag.Parse()
	if *dir == "" {
		log.Fatal("-dir is required")
	}
	if err := catalog.ImportGifs(*dsn, *dir, catalog.GifImportOptions{DryRun: *dry}); err != nil {
		log.Fatal(err)
	}
}
