package catalog

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

// foreignSeedNames are the placeholder packs. They stay fetchable by short
// name. The sticker panel does not list them, and DropForeignSeeds clears
// any featured or install rows a previous process wrote.
func foreignSeedNames() []any {
	return []any{ShortTwemoji, ShortFluent, ShortNoto}
}

// DropForeignSeeds unfeatures and uninstalls Twemoji, Fluent Emoji, and
// Noto Emoji. Running it again changes nothing. Deploy the build that
// hides those packs before restarting bff, or the old process will mark
// them featured again on startup.
func DropForeignSeeds(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return fmt.Errorf("TEAMGRAM_MYSQL_DSN or -dsn is required")
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	names := foreignSeedNames()
	in := "?,?,?"
	res, err := db.ExecContext(ctx, `UPDATE tellomi_sticker_packs SET featured=0, installed_default=0 WHERE short_name IN (`+in+`)`, names...)
	if err != nil {
		return err
	}
	packs, _ := res.RowsAffected()
	res, err = db.ExecContext(ctx, `DELETE i FROM tellomi_sticker_installs i JOIN tellomi_sticker_packs p ON p.id = i.pack_id WHERE p.short_name IN (`+in+`)`, names...)
	if err != nil {
		return err
	}
	installs, _ := res.RowsAffected()
	if _, err = db.ExecContext(ctx, `DELETE r FROM tellomi_featured_reads r JOIN tellomi_sticker_packs p ON p.id = r.pack_id WHERE p.short_name IN (`+in+`)`, names...); err != nil {
		return err
	}
	if err = bumpCatalogVersion(ctx, db, catalogStickers); err != nil {
		return err
	}
	log.Printf("importstickers: drop-foreign-seeds updated %d packs and removed %d installs", packs, installs)
	return nil
}
