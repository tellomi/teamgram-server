// Command crashreport ensures the crash-report bot and lists stored reports.
//
//	crashreport ensure -f /app/etc2/bff.yaml
//	crashreport list -f /app/etc2/bff.yaml -from 1762736400 -to 1762822800 -sender 9 -device ios
//
// The MySQL DSN is read from the bff config (ChannelMySQL) or from
// TELLOMI_CRASH_REPORT_MYSQL when that is set. The HTTP admin token is not
// accepted on the command line.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/configload"
	"github.com/teamgram/teamgram-server/pkg/crashreport"
)

type bffFile struct {
	ChannelMySQL string `json:",optional"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "ensure":
		runEnsure(os.Args[2:])
	case "list":
		runList(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runEnsure(args []string) {
	fs := flag.NewFlagSet("ensure", flag.ExitOnError)
	configFile := fs.String("f", "/app/etc2/bff.yaml", "bff config, for the MySQL DSN")
	_ = fs.Parse(args)
	db, err := crashreport.OpenMySQL(dsn(*configFile))
	if err != nil || db == nil {
		fmt.Fprintf(os.Stderr, "crash report: mysql is unavailable\n")
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = crashreport.Ensure(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "crash report: ensure failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	configFile := fs.String("f", "/app/etc2/bff.yaml", "bff config, for the MySQL DSN")
	from := fs.Int64("from", 0, "unix seconds, inclusive")
	to := fs.Int64("to", 0, "unix seconds, inclusive")
	sender := fs.Int64("sender", 0, "sender user id")
	device := fs.String("device", "", "substring matched against device, app, filename, and text")
	limit := fs.Int("limit", 50, "maximum rows, 1..200")
	_ = fs.Parse(args)
	db, err := crashreport.OpenMySQL(dsn(*configFile))
	if err != nil || db == nil {
		fmt.Fprintf(os.Stderr, "crash report: mysql is unavailable\n")
		os.Exit(1)
	}
	defer db.Close()
	objs, err := crashreport.OpenObjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "crash report: object storage is unavailable\n")
		os.Exit(1)
	}
	q := crashreport.Query{Sender: *sender, Device: *device, Limit: *limit}
	if *from > 0 {
		q.From = time.Unix(*from, 0).UTC()
	}
	if *to > 0 {
		q.To = time.Unix(*to, 0).UTC()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := crashreport.New(db, objs).List(ctx, q)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crash report: list failed: %v\n", err)
		os.Exit(1)
	}
	if rows == nil {
		rows = []crashreport.Row{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(struct {
		Reports []crashreport.Row `json:"reports"`
	}{Reports: rows}); err != nil {
		fmt.Fprintf(os.Stderr, "crash report: %v\n", err)
		os.Exit(1)
	}
}

func dsn(configFile string) string {
	if v := strings.TrimSpace(os.Getenv("TELLOMI_CRASH_REPORT_MYSQL")); v != "" {
		return v
	}
	var cfg bffFile
	configload.MustLoad(configFile, &cfg)
	return cfg.ChannelMySQL
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage:\n  crashreport ensure [-f bff.yaml]\n  crashreport list [-f bff.yaml] [-from unix] [-to unix] [-sender id] [-device text] [-limit n]\n")
}
