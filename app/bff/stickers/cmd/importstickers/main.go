// Command importstickers upserts the built-in openly licensed sticker packs.
// With -dir it also imports prepared packs from a directory of manifests.
// It is safe to run more than once. Set TEAMGRAM_MYSQL_DSN (or -dsn) and the
// TELLOMI_S3_* variables from docs/storage.md so document bytes land in R2 or MinIO.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("TEAMGRAM_MYSQL_DSN"), "MySQL DSN; defaults to TEAMGRAM_MYSQL_DSN")
	dir := flag.String("dir", "", "directory of prepared sticker packs, each with a manifest.json")
	dry := flag.Bool("dry-run", false, "validate -dir and print the plan without writing")
	featured := flag.Bool("featured", true, "feature every pack in -dir (default); -featured=false leaves the manifest flag")
	installed := flag.Bool("installed-default", true, "install every pack in -dir for every user (default); -installed-default=false leaves the manifest flag")
	dropForeign := flag.Bool("drop-foreign-seeds", false, "unfeature and uninstall Twemoji, Fluent Emoji, and Noto Emoji; safe to repeat")
	flag.Parse()
	if *dropForeign {
		if err := catalog.DropForeignSeeds(*dsn); err != nil {
			log.Fatal(err)
		}
		if *dir == "" {
			return
		}
	}
	if *dir != "" {
		err := catalog.ImportPrepared(*dsn, *dir, catalog.ImportOptions{
			DryRun:           *dry,
			Featured:         *featured,
			InstalledDefault: *installed,
		})
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if *dry {
		log.Fatal(fmt.Errorf("-dry-run requires -dir"))
	}
	if err := catalog.Import(*dsn); err != nil {
		log.Fatal(err)
	}
}
