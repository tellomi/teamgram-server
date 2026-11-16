package persist_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
	"github.com/teamgram/teamgram-server/app/bff/passkey/persist"
	"google.golang.org/grpc/status"

	_ "github.com/go-sql-driver/mysql"
)

func TestPasskeyStoredAndVerified(t *testing.T) {
	dsn := openPasskeyMySQL(t)
	applyPasskeySQL(t, dsn)
	db := mustOpen(t, dsn)
	persist.Set(persist.OpenDB(db))
	t.Cleanup(func() { persist.Set(nil) })

	const userID int64 = 42
	const origin = "https://www.tellomi.cn"
	const regKey int64 = 11
	const loginKey int64 = 22
	opts, err := accountrest.InitPasskeyRegistration(userID, regKey)
	if err != nil {
		t.Fatal(err)
	}
	raw := opts.GetOptions().GetData()
	if !strings.Contains(raw, `"publicKey"`) || !strings.Contains(raw, `"id":"www.tellomi.cn"`) || !strings.Contains(raw, `"userVerification":"required"`) {
		t.Fatal(raw)
	}
	challenge, userB64 := challengeOf(t, raw)
	handle, err := base64.RawURLEncoding.DecodeString(userB64)
	if err != nil || string(handle) != "1:42" {
		t.Fatalf("user handle %q err %v", handle, err)
	}
	key := newSoftKey(t, "www.tellomi.cn", handle)
	if _, err = accountrest.RegisterPasskey(userID, regKey, key.register(t, "not-the-challenge", origin)); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatal(err)
	}
	if _, err = accountrest.RegisterPasskey(userID, regKey, key.register(t, challenge, "https://evil.example")); codeOf(err) != "PASSKEY_ORIGIN_MISMATCH" {
		t.Fatal(err)
	}
	good := key.register(t, challenge, origin)
	if _, err = accountrest.RegisterPasskey(userID, regKey+1, good); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatalf("other auth key %v", err)
	}
	saved, err := accountrest.RegisterPasskey(userID, regKey, good)
	if err != nil || saved.GetId() != key.idString() || saved.GetName() != "Passkey" {
		t.Fatalf("register %v %#v", err, saved)
	}
	listed, err := accountrest.ListPasskeys(userID)
	if err != nil || len(listed.GetPasskeys()) != 1 || listed.GetPasskeys()[0].GetId() != key.idString() {
		t.Fatalf("list %v %#v", err, listed)
	}

	// A second pool is a new process: it has no in-memory credential.
	again := mustOpen(t, dsn)
	persist.Set(persist.OpenDB(again))
	login, err := accountrest.InitPasskeyLogin(4, "hash", loginKey)
	if err != nil || !strings.Contains(login.GetOptions().GetData(), "www.tellomi.cn") || !strings.Contains(login.GetOptions().GetData(), `"userVerification":"required"`) {
		t.Fatal(err, login)
	}
	loginChallenge, _ := challengeOf(t, login.GetOptions().GetData())
	assertion := key.login(t, loginChallenge, origin)
	if _, err = accountrest.VerifyPasskeyLogin(loginKey+1, assertion); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatalf("other login key %v", err)
	}
	got, err := accountrest.VerifyPasskeyLogin(loginKey, assertion)
	if err != nil || got != userID {
		t.Fatalf("login user %d err %v", got, err)
	}
	listed, err = accountrest.ListPasskeys(userID)
	if err != nil || listed.GetPasskeys()[0].GetLastUsageDate().GetValue() == 0 {
		t.Fatalf("last used %v %#v", err, listed)
	}

	login, err = accountrest.InitPasskeyLogin(4, "hash", loginKey)
	if err != nil {
		t.Fatal(err)
	}
	loginChallenge, _ = challengeOf(t, login.GetOptions().GetData())
	replay := key.loginAt(t, loginChallenge, origin, 1)
	if _, err = accountrest.VerifyPasskeyLogin(loginKey, replay); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatalf("replay %v", err)
	}
	login, err = accountrest.InitPasskeyLogin(4, "hash", loginKey)
	if err != nil {
		t.Fatal(err)
	}
	loginChallenge, _ = challengeOf(t, login.GetOptions().GetData())
	noUV := key.loginFlags(t, loginChallenge, origin, 0x01)
	if _, err = accountrest.VerifyPasskeyLogin(loginKey, noUV); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatalf("missing UV %v", err)
	}
	login, err = accountrest.InitPasskeyLogin(4, "hash", loginKey)
	if err != nil {
		t.Fatal(err)
	}
	loginChallenge, _ = challengeOf(t, login.GetOptions().GetData())
	bad := key.login(t, loginChallenge, origin)
	bad.GetResponse().Signature[0] ^= 0xff
	if _, err = accountrest.VerifyPasskeyLogin(loginKey, bad); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatalf("bad signature %v", err)
	}
	login, err = accountrest.InitPasskeyLogin(4, "hash", loginKey)
	if err != nil {
		t.Fatal(err)
	}
	loginChallenge, _ = challengeOf(t, login.GetOptions().GetData())
	unknown := key.login(t, loginChallenge, origin)
	unknown.Id = base64.RawURLEncoding.EncodeToString([]byte("not-this-passkey!!"))
	unknown.RawId = unknown.Id
	if _, err = accountrest.VerifyPasskeyLogin(loginKey, unknown); codeOf(err) != "PASSKEY_CREDENTIAL_NOT_FOUND" {
		t.Fatalf("unknown %v", err)
	}
	againReg, err := accountrest.InitPasskeyRegistration(userID, regKey)
	if err != nil {
		t.Fatal(err)
	}
	againChallenge, _ := challengeOf(t, againReg.GetOptions().GetData())
	if _, err = accountrest.RegisterPasskey(userID, regKey, key.register(t, againChallenge, origin)); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatalf("duplicate raw id %v", err)
	}

	third := mustOpen(t, dsn)
	persist.Set(persist.OpenDB(third))
	login, err = accountrest.InitPasskeyLogin(4, "hash", loginKey)
	if err != nil {
		t.Fatal(err)
	}
	loginChallenge, _ = challengeOf(t, login.GetOptions().GetData())
	got, err = accountrest.VerifyPasskeyLogin(loginKey, key.login(t, loginChallenge, origin))
	if err != nil || got != userID {
		t.Fatalf("third process user %d err %v", got, err)
	}
	gone, err := accountrest.DeletePasskey(userID, key.idString())
	if err != nil || !mtproto.FromBool(gone) {
		t.Fatal(err, gone)
	}
	if _, err = accountrest.VerifyPasskeyLogin(loginKey, unknown); codeOf(err) != "PASSKEY_CREDENTIAL_NOT_FOUND" {
		t.Fatal(err)
	}
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func applyPasskeySQL(t *testing.T, dsn string) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "teamgramd", "deploy", "sql", "migrate-20261115-passkeys.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db := mustOpen(t, dsn)
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = stripSQLComments(stmt)
		if stmt == "" {
			continue
		}
		if _, err = db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func stripSQLComments(stmt string) string {
	var b strings.Builder
	for _, line := range strings.Split(stmt, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func mustOpen(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func openPasskeyMySQL(t *testing.T) string {
	t.Helper()
	admin, err := passkeyAdminDSN(t)
	if err != nil {
		if os.Getenv("GITHUB_ACTIONS") == "1" {
			t.Fatalf("mysql is required in CI: %v", err)
		}
		t.Skipf("mysql not available: %v", err)
	}
	db, err := sql.Open("mysql", admin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schema := fmt.Sprintf("passkey_%d", os.Getpid())
	if _, err = db.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE IF EXISTS " + schema) })
	return dsnWithSchema(admin, schema)
}

func passkeyAdminDSN(t *testing.T) (string, error) {
	if dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); dsn != "" && pingMySQL(dsn) {
		return dsn, nil
	}
	for _, dsn := range []string{
		"teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:root@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
	} {
		if pingMySQL(dsn) {
			return dsn, nil
		}
	}
	port, err := startPasskeyMySQL(t)
	if err != nil {
		return "", err
	}
	dsn := fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/mysql?charset=utf8mb4&parseTime=true", port)
	if !pingMySQL(dsn) {
		return "", errors.New("started mysql did not answer")
	}
	return dsn, nil
}

func pingMySQL(dsn string) bool {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return db.PingContext(ctx) == nil
}

func dsnWithSchema(dsn, schema string) string {
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

func startPasskeyMySQL(t *testing.T) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", err
	}
	name := fmt.Sprintf("teamgram-passkey-%d", os.Getpid())
	_ = exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"-e", "MYSQL_ROOT_PASSWORD=root",
		"-e", "MYSQL_ROOT_HOST=%",
		"-p", "127.0.0.1::3306",
		"mysql:8.0.46").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %w %s", err, out)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		probe := exec.Command("docker", "exec", name, "mysqladmin", "ping", "-uroot", "-proot", "--silent")
		if probe.Run() == nil {
			portOut, portErr := exec.Command("docker", "port", name, "3306").CombinedOutput()
			if portErr != nil {
				return "", portErr
			}
			fields := strings.Fields(string(portOut))
			if len(fields) == 0 {
				return "", fmt.Errorf("docker port %q", portOut)
			}
			hostport := fields[len(fields)-1]
			return hostport[strings.LastIndex(hostport, ":")+1:], nil
		}
		time.Sleep(time.Second)
	}
	return "", errors.New("mysql container did not become ready")
}
