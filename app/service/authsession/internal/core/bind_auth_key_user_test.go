package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/svc"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"

	_ "github.com/go-sql-driver/mysql"
)

// The handler, not the dao, turns a temp key into the permanent key before
// the bind. A second user on that temp key must fail and leave the permanent
// row unchanged.
func TestBindAuthKeyUserNormalizesTempKey(t *testing.T) {
	db, dsn := openBindMySQL(t)
	mr := miniredis.RunT(t)
	cfg := config.Config{}
	cfg.Mysql.DSN = dsn
	cfg.Cache = cache.ClusterConf{{
		RedisConf: redis.RedisConf{Host: mr.Addr(), Type: redis.NodeType},
		Weight:    100,
	}}
	cfg.KV = cfg.Cache
	d := dao.New(cfg)
	c := New(context.Background(), &svc.ServiceContext{Dao: d})

	const perm, temp, userID, other int64 = 501, 502, 601, 602
	insertAuthKey(t, db, perm, 0, perm)
	insertAuthKey(t, db, temp, 1, perm)

	reply, err := c.AuthsessionBindAuthKeyUser(&authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: temp,
		UserId:    userID,
	})
	if err != nil || reply.GetV() == 0 {
		t.Fatalf("bind via temp key: %v %v", reply, err)
	}
	if n := countUsers(t, db, temp); n != 0 {
		t.Fatalf("temp key has %d auth_users rows", n)
	}
	owner, hash, deleted := oneUser(t, db, perm)
	if owner != userID || deleted != 0 || hash == 0 {
		t.Fatalf("perm row user %d hash %d deleted %d", owner, hash, deleted)
	}

	_, err = c.AuthsessionBindAuthKeyUser(&authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: temp,
		UserId:    other,
	})
	if !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("second owner err=%v", err)
	}
	rpc := mtproto.NewRpcError(err)
	if rpc.GetErrorCode() != 401 || rpc.GetErrorMessage() != "AUTH_KEY_UNREGISTERED" {
		t.Fatalf("client would see code=%d message=%s", rpc.GetErrorCode(), rpc.GetErrorMessage())
	}
	owner2, hash2, deleted2 := oneUser(t, db, perm)
	if owner2 != owner || hash2 != hash || deleted2 != deleted {
		t.Fatalf("conflict rewrote perm row user %d hash %d deleted %d", owner2, hash2, deleted2)
	}
	if n := countUsers(t, db, temp); n != 0 {
		t.Fatalf("conflict wrote the temp key, rows %d", n)
	}
}

func insertAuthKey(t *testing.T, db *sql.DB, id int64, keyType int, permID int64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO auth_keys (auth_key_id, body) VALUES (?, ?)`, id, "AQID"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth_key_infos (auth_key_id, auth_key_type, perm_auth_key_id) VALUES (?, ?, ?)`, id, keyType, permID); err != nil {
		t.Fatal(err)
	}
}

func countUsers(t *testing.T, db *sql.DB, authKeyID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM auth_users WHERE auth_key_id = ?`, authKeyID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func oneUser(t *testing.T, db *sql.DB, authKeyID int64) (userID, hash int64, deleted int) {
	t.Helper()
	err := db.QueryRow(`SELECT user_id, hash, deleted FROM auth_users WHERE auth_key_id = ?`, authKeyID).Scan(&userID, &hash, &deleted)
	if err != nil {
		t.Fatal(err)
	}
	return userID, hash, deleted
}

func openBindMySQL(t *testing.T) (*sql.DB, string) {
	t.Helper()
	candidates := []string{
		os.Getenv("TEAMGRAM_MYSQL_DSN"),
		"teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:root@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
	}
	var adminDSN string
	for _, dsn := range candidates {
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			continue
		}
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			continue
		}
		err = pingMySQL(db)
		if err != nil {
			_ = db.Close()
			continue
		}
		adminDSN = dsn
		_ = db.Close()
		break
	}
	if adminDSN == "" {
		if port, err := startBindMySQL(t); err == nil {
			adminDSN = fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/mysql?charset=utf8mb4&parseTime=true", port)
		}
	}
	if adminDSN == "" {
		if os.Getenv("GITHUB_ACTIONS") == "1" {
			t.Fatal("mysql is required in CI")
		}
		t.Skip("mysql not available")
	}
	admin, err := sql.Open("mysql", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("auth_bind_core_%d", os.Getpid())
	if err := execMySQL(admin, "DROP DATABASE IF EXISTS "+schema); err != nil {
		t.Fatal(err)
	}
	if err := execMySQL(admin, "CREATE DATABASE "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + schema) })
	dsn := bindDSNWithSchema(adminDSN, schema)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range strings.Split(bindSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := execMySQL(db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db, dsn
}

func startBindMySQL(t *testing.T) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		return "", err
	}
	name := fmt.Sprintf("teamgram-auth-bind-core-%d", os.Getpid())
	_ = exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"-e", "MYSQL_ROOT_PASSWORD=root",
		"-e", "MYSQL_ROOT_HOST=%",
		"-p", "127.0.0.1::3306",
		"mysql:8.0.46").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w %s", err, out)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		probe, probeErr := exec.Command("docker", "exec", name, "mysql", "-uroot", "-proot", "-N", "-e", "SELECT 1").CombinedOutput()
		if probeErr == nil && strings.Contains(string(probe), "1") {
			portOut, portErr := exec.Command("docker", "port", name, "3306").CombinedOutput()
			if portErr != nil {
				return "", portErr
			}
			fields := strings.Fields(string(portOut))
			if len(fields) == 0 {
				return "", fmt.Errorf("docker port %q", portOut)
			}
			hostport := fields[len(fields)-1]
			port := hostport[strings.LastIndex(hostport, ":")+1:]
			probeDSN := fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/mysql?charset=utf8mb4&parseTime=true", port)
			probeDB, openErr := sql.Open("mysql", probeDSN)
			if openErr != nil {
				return "", openErr
			}
			pingErr := pingMySQL(probeDB)
			_ = probeDB.Close()
			if pingErr != nil {
				time.Sleep(time.Second)
				continue
			}
			return port, nil
		}
		time.Sleep(time.Second)
	}
	return "", errors.New("mysql container did not become ready")
}

func pingMySQL(db *sql.DB) error {
	var err error
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil || !mysqlInvalid(err) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
	}
	return err
}

func execMySQL(db *sql.DB, query string) error {
	var err error
	for i := 0; i < 5; i++ {
		_, err = db.Exec(query)
		if err == nil || !mysqlInvalid(err) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
	}
	return err
}

func mysqlInvalid(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid connection")
}

func bindDSNWithSchema(dsn, schema string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return dsn + "/" + schema
	}
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return dsn[:slash+1] + schema + rest[q:]
	}
	return dsn[:slash+1] + schema
}

const bindSchema = `
CREATE TABLE auth_keys (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  body VARCHAR(512) NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
) ENGINE=InnoDB;
CREATE TABLE auth_key_infos (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  auth_key_type INT NOT NULL,
  perm_auth_key_id BIGINT NOT NULL DEFAULT 0,
  temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
  media_temp_auth_key_id BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
) ENGINE=InnoDB;
CREATE TABLE auth_users (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL DEFAULT 0,
  hash BIGINT NOT NULL DEFAULT 0,
  date_created BIGINT NOT NULL DEFAULT 0,
  date_active BIGINT NOT NULL DEFAULT 0,
  android_push_session_id BIGINT NOT NULL DEFAULT 0,
  encrypted_requests_disabled TINYINT(1) NOT NULL DEFAULT 0,
  call_requests_disabled TINYINT(1) NOT NULL DEFAULT 0,
  unconfirmed TINYINT(1) NOT NULL DEFAULT 0,
  deleted TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id, user_id)
) ENGINE=InnoDB;
CREATE TABLE auths (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  layer INT NOT NULL DEFAULT 0,
  api_id INT NOT NULL DEFAULT 0,
  device_model VARCHAR(128) NOT NULL DEFAULT '',
  system_version VARCHAR(64) NOT NULL DEFAULT '',
  app_version VARCHAR(64) NOT NULL DEFAULT '',
  system_lang_code VARCHAR(64) NOT NULL DEFAULT '',
  lang_pack VARCHAR(64) NOT NULL DEFAULT '',
  lang_code VARCHAR(64) NOT NULL DEFAULT '',
  client_ip VARCHAR(64) NOT NULL DEFAULT '',
  proxy VARCHAR(64) NOT NULL DEFAULT '',
  params VARCHAR(512) NOT NULL DEFAULT '',
  date_active BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
) ENGINE=InnoDB
`
