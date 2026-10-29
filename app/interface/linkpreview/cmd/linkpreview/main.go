package main

import (
	"flag"
	"log"

	"github.com/teamgram/teamgram-server/app/interface/linkpreview"
)

func main() {
	configFile := flag.String("f", "", "yaml config file")
	flag.Parse()

	cfg := linkpreview.Load(*configFile)
	var cat linkpreview.Catalog
	if cfg.MysqlDSN != "" {
		mysql, err := linkpreview.OpenMySQL(cfg.MysqlDSN)
		if err != nil {
			log.Fatalf("linkpreview: mysql: %v", err)
		}
		defer mysql.Close()
		cat = mysql
	} else {
		log.Printf("linkpreview: no MySQL DSN; proxy, socks, share, and /boost still resolve")
		cat = linkpreview.NewMemory()
	}
	if err := linkpreview.NewServer(cfg, cat).ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
