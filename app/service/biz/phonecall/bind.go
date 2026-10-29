// Package phonecall shares the active group-call binding between the bff
// process (which runs the call) and the biz process (which fills chat.call).
package phonecall

import (
	"context"
	"database/sql"
	"os"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

const schema = `
CREATE TABLE IF NOT EXISTS phone_peer_call (
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  call_id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  active TINYINT NOT NULL,
  not_empty TINYINT NOT NULL,
  PRIMARY KEY (peer_type, peer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

type row struct {
	callID, access   int64
	active, notEmpty bool
}

var (
	mu   sync.Mutex
	mem  = map[[2]int64]row{}
	db   *sql.DB
	once sync.Once
)

// UseDSN opens MySQL so another process can read the binding. An empty dsn
// keeps the in-process map only. TEAMGRAM_MYSQL_DSN overrides dsn.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	if err = conn.Ping(); err != nil {
		_ = conn.Close()
		return err
	}
	if _, err = conn.Exec(schema); err != nil {
		_ = conn.Close()
		return err
	}
	mu.Lock()
	if db != nil {
		_ = db.Close()
	}
	db = conn
	mu.Unlock()
	return nil
}

func key(peerType int32, peerID int64) [2]int64 {
	return [2]int64{int64(peerType), peerID}
}

// Publish records which group call a chat or channel is hosting.
func Publish(peerType int32, peerID, callID, access int64, active, notEmpty bool) {
	k := key(peerType, peerID)
	mu.Lock()
	if !active {
		delete(mem, k)
	} else {
		mem[k] = row{callID: callID, access: access, active: true, notEmpty: notEmpty}
	}
	conn := db
	mu.Unlock()
	if conn == nil {
		return
	}
	ctx := context.Background()
	if !active {
		_, _ = conn.ExecContext(ctx, `DELETE FROM phone_peer_call WHERE peer_type=? AND peer_id=?`, peerType, peerID)
		return
	}
	ne, ac := 0, 1
	if notEmpty {
		ne = 1
	}
	_, _ = conn.ExecContext(ctx, `INSERT INTO phone_peer_call (peer_type, peer_id, call_id, access_hash, active, not_empty)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE call_id=VALUES(call_id), access_hash=VALUES(access_hash), active=VALUES(active), not_empty=VALUES(not_empty)`,
		peerType, peerID, callID, access, ac, ne)
}

// Lookup returns the binding visible in this process.
func Lookup(peerType int32, peerID int64) (callID, access int64, active, notEmpty, ok bool) {
	mu.Lock()
	r, found := mem[key(peerType, peerID)]
	mu.Unlock()
	if !found {
		return 0, 0, false, false, false
	}
	return r.callID, r.access, r.active, r.notEmpty, true
}

// LookupDB reads the binding from MySQL. The biz process uses this.
func LookupDB(ctx context.Context, dsn string, peerType int32, peerID int64) (callID, access int64, active, notEmpty bool, err error) {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return 0, 0, false, false, nil
	}
	once.Do(func() { _ = UseDSN(dsn) })
	mu.Lock()
	conn := db
	mu.Unlock()
	if conn == nil {
		return 0, 0, false, false, nil
	}
	var ac, ne int
	err = conn.QueryRowContext(ctx, `SELECT call_id, access_hash, active, not_empty FROM phone_peer_call WHERE peer_type=? AND peer_id=?`, peerType, peerID).
		Scan(&callID, &access, &ac, &ne)
	if err == sql.ErrNoRows {
		return 0, 0, false, false, nil
	}
	if err != nil {
		return 0, 0, false, false, err
	}
	return callID, access, ac != 0, ne != 0, nil
}
