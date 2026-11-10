package secretchat

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRestartKeepsTheSecretTimer(t *testing.T) {
	be := newMem()
	admin := New(be)
	waiting, _, err := admin.Request(1, 2, 7, []byte{1}, 10)
	if err != nil {
		t.Fatal(err)
	}
	peer := New(be)
	if _, _, err = peer.Accept(2, input(waiting), []byte{2}, 3, 11); err != nil {
		t.Fatal(err)
	}
	in := input(waiting)
	if err = admin.SetTTL(1, in, 5); err != nil {
		t.Fatal(err)
	}
	// A new store on the same backend is a restart. The timer has to come
	// back from the saved chat, not from this process's memory.
	restarted := New(be)
	if _, _, err = restarted.Send(1, in, 77, []byte("cipher"), nil, false, 20); err != nil {
		t.Fatal(err)
	}
	loaded, ok := be.Chat(waiting.GetId())
	if !ok || loaded.ttl != 5 {
		t.Fatalf("ttl after restart = %d ok=%v", loaded.ttl, ok)
	}
}

func TestScanChatReadsTTL(t *testing.T) {
	c, err := scanChat(fixedChatScan{})
	if err != nil {
		t.Fatal(err)
	}
	if c.ttl != 15 || c.id != 4 || !c.historyGone {
		t.Fatalf("scanned %+v", c)
	}
}

type fixedChatScan struct{}

func (fixedChatScan) Scan(dest ...any) error {
	vals := []any{int32(4), int64(9), int32(10), int64(1), int64(2), []byte{1}, []byte{2}, int64(3), 1, 1, int32(15)}
	if len(dest) != len(vals) {
		return errScanLen
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(vals[i]))
	}
	return nil
}

var errScanLen = errString("scan len")

type errString string

func (e errString) Error() string { return string(e) }

func TestSecretTTLMigrationFollowsTheTable(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "teamgramd", "deploy", "sql")
	table, err := os.ReadFile(filepath.Join(dir, "migrate-20261120-secret-qts.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(table), "CREATE TABLE IF NOT EXISTS `secret_chats`") {
		t.Fatal("secret_chats table migration moved")
	}
	later, err := os.ReadFile(filepath.Join(dir, "migrate-20261124-secret-chat-ttl.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(later)
	if !strings.Contains(text, "teamgram_add_column('secret_chats', 'ttl_period'") {
		t.Fatal("ttl column add is not in the later migration")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, "message-expiry") && name < "migrate-20261120-secret-qts.sql" {
			t.Fatalf("%s alters secret_chats before the table exists", name)
		}
	}
}
