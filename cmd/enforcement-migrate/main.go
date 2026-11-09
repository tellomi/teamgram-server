// Command enforcement-migrate creates the enforcement tables in the database
// named by TELLOMI_ENFORCEMENT_MYSQL. The operator runs it once. It does not
// create the database, it does not run at process start, and it refuses the
// chat database name teamgram.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/enforcement"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv(enforcement.EnvMySQL))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "enforcement-migrate: TELLOMI_ENFORCEMENT_MYSQL is empty")
		os.Exit(1)
	}
	if err := enforcement.Migrate(dsn); err != nil {
		fmt.Fprintln(os.Stderr, "enforcement-migrate:", err.Error())
		os.Exit(1)
	}
	fmt.Println("enforcement-migrate: tables are ready")
}
