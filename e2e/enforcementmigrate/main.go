// Command enforcementmigrate creates the isolated enforcement database and
// its tables. It is the explicit Migrate step for the client regression job.
// It refuses any host other than 127.0.0.1 or localhost, and it refuses every
// database name except tellomi_enforcement_e2e.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/pkg/enforcement"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv("TELLOMI_ENFORCEMENT_MYSQL"))
	if err := run(dsn); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(dsn string) error {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("enforcement migrate: dsn")
	}
	switch cfg.Addr {
	case "127.0.0.1:3306", "localhost:3306":
	default:
		return fmt.Errorf("enforcement migrate: host must be local")
	}
	if cfg.DBName != "tellomi_enforcement_e2e" {
		return fmt.Errorf("enforcement migrate: database must be tellomi_enforcement_e2e")
	}
	admin := *cfg
	admin.DBName = "mysql"
	db, err := mysqltls.Open(admin.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		return err
	}
	if _, err = db.Exec("CREATE DATABASE IF NOT EXISTS `tellomi_enforcement_e2e` CHARACTER SET utf8mb4"); err != nil {
		return err
	}
	return enforcement.Migrate(dsn)
}
