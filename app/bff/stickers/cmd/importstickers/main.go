// Command importstickers upserts the built-in openly licensed sticker packs.
// It is safe to run more than once. Set TEAMGRAM_MYSQL_DSN (or -dsn) and the
// TELLOMI_S3_* variables from docs/storage.md so document bytes land in R2 or MinIO.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("TEAMGRAM_MYSQL_DSN"), "MySQL DSN; defaults to TEAMGRAM_MYSQL_DSN")
	flag.Parse()
	if err := catalog.Import(*dsn); err != nil {
		log.Fatal(err)
	}
}
