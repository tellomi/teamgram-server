// Command accesshashbackfill assigns a non-zero random access_hash to user
// and channel rows that still store 0.
//
//	accesshashbackfill --dsn 'user:pass@tcp(127.0.0.1:3306)/teamgram'
//
// Without --apply it only prints how many rows are 0. --apply updates those
// rows. Nothing in deploy or process startup runs this command. Run it by
// hand before TELLOMI_ACCESS_HASH_ENFORCE is set to log or enforce, then
// let clients refresh the new values from user and channel objects.
//
// The DSN is not printed. A row whose access_hash changes between the read
// and the update is left alone.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN for the teamgram database")
	apply := flag.Bool("apply", false, "write non-zero hashes; without this flag only the counts are printed")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: --dsn is required")
		os.Exit(2)
	}
	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: open:", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: ping:", err)
		os.Exit(1)
	}
	users, err := countZero(ctx, db, "users")
	if err != nil {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: users:", err)
		os.Exit(1)
	}
	channels, err := countZero(ctx, db, "channels")
	if err != nil {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: channels:", err)
		os.Exit(1)
	}
	fmt.Printf("users=%d channels=%d\n", users, channels)
	if !*apply {
		fmt.Println("dry-run")
		return
	}
	n, err := fillZero(ctx, db, "users")
	if err != nil {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: users:", err)
		os.Exit(1)
	}
	m, err := fillZero(ctx, db, "channels")
	if err != nil {
		fmt.Fprintln(os.Stderr, "accesshashbackfill: channels:", err)
		os.Exit(1)
	}
	fmt.Printf("updated users=%d channels=%d\n", n, m)
}

func countZero(ctx context.Context, db *sql.DB, table string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE access_hash = 0").Scan(&n)
	return n, err
}

func fillZero(ctx context.Context, db *sql.DB, table string) (int, error) {
	rows, err := db.QueryContext(ctx, "SELECT id FROM "+table+" WHERE access_hash = 0")
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	updated := 0
	stmt := "UPDATE " + table + " SET access_hash = ? WHERE id = ? AND access_hash = 0"
	for _, id := range ids {
		hash, err := nextHash()
		if err != nil {
			return updated, err
		}
		res, err := db.ExecContext(ctx, stmt, hash, id)
		if err != nil {
			return updated, err
		}
		n, _ := res.RowsAffected()
		updated += int(n)
	}
	return updated, nil
}

func nextHash() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1, nil
	}
	return v, nil
}
