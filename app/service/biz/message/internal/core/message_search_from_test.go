package core

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"

	_ "github.com/go-sql-driver/mysql"
)

const (
	searchCaller int64 = 930001
	searchOther  int64 = 930002
	searchChat   int64 = 880301
)

func TestMessageSearchFromIDFilters(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_DSN") != "" {
		t.Fatal("refusing TEAMGRAM_MYSQL_DSN")
	}
	raw, dsn := openSearchMySQL(t)
	defer raw.Close()
	db := sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
	ctx := context.Background()
	c := New(ctx, &svc.ServiceContext{Dao: &dao.Dao{Mysql: &dao.Mysql{
		DB:          db,
		MessagesDAO: mysql_dao.NewMessagesDAO(db, 0),
		CommonDAO:   sqlx.NewCommonDAO(db),
	}}})
	c.Logger = logx.WithContext(ctx)

	dialog := mtproto.MakeDialogId(searchCaller, mtproto.PEER_USER, searchOther)
	chatDialog := mtproto.MakeDialogId(searchCaller, mtproto.PEER_CHAT, searchChat)
	insert := func(user, box, sender, d1, d2 int64, peerType int32, peer int64, text, data string, date int64, deleted int) {
		t.Helper()
		if data == "" {
			data = `{"message":"` + text + `"}`
		}
		_, err := raw.ExecContext(ctx, `INSERT INTO messages
			(user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, message_data, message, date2, deleted)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			user, box, d1, d2, box, sender, peerType, peer, data, text, date, deleted)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(searchCaller, 10, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "apple banana", "", 100, 0)
	insert(searchCaller, 11, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "orange only", "", 500, 0)
	insert(searchCaller, 12, searchOther, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "apple banana", "", 500, 0)
	insert(searchCaller, 13, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "apple banana", "", 100, 1)
	insert(searchCaller, 20, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "apple banana", "", 600, 0)
	insert(searchCaller, 21, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "", `{"message":"kiwi only"}`, 200, 0)
	insert(searchCaller, 22, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "100%", "", 150, 0)
	insert(searchCaller, 23, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchOther, "100X", "", 150, 0)
	insert(searchCaller, 30, searchCaller, chatDialog.A, chatDialog.B, mtproto.PEER_CHAT, searchChat, "apple banana", "", 300, 0)
	insert(searchOther, 40, searchCaller, dialog.A, dialog.B, mtproto.PEER_USER, searchCaller, "apple banana", "", 100, 0)

	got := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 0, 0, 0, 20)
	if ids := boxIDs(got); !sameIDs(ids, []int32{20, 10}) || got.Count != 2 {
		t.Fatalf("from+q = %v count %d", ids, got.Count)
	}
	page := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 0, 0, 0, 1)
	if ids := boxIDs(page); !sameIDs(ids, []int32{20}) || page.Count != 2 {
		t.Fatalf("first page = %v count %d", ids, page.Count)
	}
	next := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 0, 0, 20, 1)
	if ids := boxIDs(next); !sameIDs(ids, []int32{10}) || next.Count != 2 {
		t.Fatalf("second page = %v count %d", ids, next.Count)
	}
	dated := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 400, 0, 0, 20)
	if ids := boxIDs(dated); !sameIDs(ids, []int32{20}) || dated.Count != 1 {
		t.Fatalf("min_date 400 = %v count %d", ids, dated.Count)
	}
	empty := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 700, 0, 0, 20)
	if ids := boxIDs(empty); len(ids) != 0 || empty.Count != 0 {
		t.Fatalf("min_date 700 = %v count %d", ids, empty.Count)
	}
	maxed := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 0, 100, 0, 20)
	if ids := boxIDs(maxed); !sameIDs(ids, []int32{10}) || maxed.Count != 1 {
		t.Fatalf("max_date 100 = %v count %d", ids, maxed.Count)
	}
	under := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "banana apple", 0, 99, 0, 20)
	if ids := boxIDs(under); len(ids) != 0 || under.Count != 0 {
		t.Fatalf("max_date 99 = %v count %d", ids, under.Count)
	}
	folded := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "BANANA APPLE", 0, 0, 0, 20)
	if ids := boxIDs(folded); !sameIDs(ids, []int32{20, 10}) {
		t.Fatalf("case = %v", ids)
	}
	escaped := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "100%", 0, 0, 0, 20)
	if ids := boxIDs(escaped); !sameIDs(ids, []int32{22}) || escaped.Count != 1 {
		t.Fatalf("escaped percent = %v count %d", ids, escaped.Count)
	}
	fallback := searchBoxes(t, c, searchCaller, mtproto.PEER_USER, searchOther, searchCaller, "kiwi", 0, 0, 0, 20)
	if ids := boxIDs(fallback); !sameIDs(ids, []int32{21}) || fallback.Count != 1 {
		t.Fatalf("message_data fallback = %v count %d", ids, fallback.Count)
	}
	otherBox := searchBoxes(t, c, searchOther, mtproto.PEER_USER, searchCaller, searchCaller, "banana apple", 0, 0, 0, 20)
	if ids := boxIDs(otherBox); !sameIDs(ids, []int32{40}) || otherBox.Count != 1 {
		t.Fatalf("other mailbox = %v count %d", ids, otherBox.Count)
	}
	chatHit := searchBoxes(t, c, searchCaller, mtproto.PEER_CHAT, searchChat, searchCaller, "banana apple", 0, 0, 0, 20)
	if ids := boxIDs(chatHit); !sameIDs(ids, []int32{30}) || chatHit.Count != 1 {
		t.Fatalf("basic group = %v count %d", ids, chatHit.Count)
	}
}

func searchBoxes(t *testing.T, c *MessageCore, user int64, peerType int32, peer, from int64, q string, minDate, maxDate, offset, limit int32) *mtproto.MessageBoxList {
	t.Helper()
	out, err := c.MessageSearchV2(&message.TLMessageSearchV2{
		UserId:   user,
		PeerType: peerType,
		PeerId:   peer,
		Q:        q,
		FromId:   from,
		MinDate:  minDate,
		MaxDate:  maxDate,
		OffsetId: offset,
		Limit:    limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func boxIDs(list *mtproto.MessageBoxList) []int32 {
	out := make([]int32, 0, len(list.GetBoxList()))
	for _, box := range list.GetBoxList() {
		out = append(out, box.GetMessageId())
	}
	return out
}

func sameIDs(got, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func searchDSN(base, schema string) string {
	return base + schema + "?charset=utf8mb4&parseTime=true&loc=Local"
}

// openSearchMySQL uses a loopback server only. go test ./... has no MySQL,
// and a refused port is a skip, the same as the other loopback schema tests.
// TEAMGRAM_REQUIRE_SEARCH_MYSQL=1 is only for the job that must execute the
// assertions: a refused port is a failure there, so a green job cannot hide
// a skipped search. Once a server answers, a failed query or a wrong search
// result still fails. TEAMGRAM_MYSQL_DSN is never read.
func openSearchMySQL(t *testing.T) (*sql.DB, string) {
	t.Helper()
	candidates := []string{
		"qrtest:qrtest-local@tcp(127.0.0.1:3306)/",
		"root:@tcp(127.0.0.1:3306)/",
		"root:root@tcp(127.0.0.1:3306)/",
		"root:integration-fixture@tcp(127.0.0.1:3306)/",
	}
	var admin *sql.DB
	var base string
	var last error
	for _, candidate := range candidates {
		db, err := sql.Open("mysql", candidate+"?charset=utf8mb4&parseTime=true&loc=Local")
		if err != nil {
			last = err
			continue
		}
		if err = db.Ping(); err != nil {
			_ = db.Close()
			last = err
			continue
		}
		admin = db
		base = candidate
		break
	}
	if admin == nil {
		if os.Getenv("TEAMGRAM_REQUIRE_SEARCH_MYSQL") == "1" {
			t.Fatalf("required loopback mysql is not available: %v", last)
		}
		t.Skipf("loopback mysql not available: %v", last)
	}
	schema := fmt.Sprintf("search_from_%d", os.Getpid())
	if _, err := admin.Exec("CREATE DATABASE " + schema); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE " + schema)
		_ = admin.Close()
	})
	raw, err := sql.Open("mysql", searchDSN(base, schema))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`CREATE TABLE messages (
		id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
		user_id bigint NOT NULL,
		user_message_box_id int NOT NULL,
		dialog_id1 bigint NOT NULL,
		dialog_id2 bigint NOT NULL,
		dialog_message_id bigint NOT NULL DEFAULT 0,
		sender_user_id bigint NOT NULL,
		peer_type int NOT NULL,
		peer_id bigint NOT NULL,
		random_id bigint NOT NULL DEFAULT 0,
		message_filter_type int NOT NULL DEFAULT 0,
		message_data json DEFAULT NULL,
		message varchar(6000) NOT NULL DEFAULT '',
		mentioned tinyint NOT NULL DEFAULT 0,
		media_unread tinyint NOT NULL DEFAULT 0,
		pinned tinyint NOT NULL DEFAULT 0,
		has_reaction tinyint NOT NULL DEFAULT 0,
		reaction varchar(256) NOT NULL DEFAULT '',
		reaction_date bigint NOT NULL DEFAULT 0,
		reaction_unread tinyint NOT NULL DEFAULT 0,
		saved_peer_type int NOT NULL DEFAULT 0,
		saved_peer_id bigint NOT NULL DEFAULT 0,
		date2 bigint NOT NULL DEFAULT 0,
		ttl_period int NOT NULL DEFAULT 0,
		deleted tinyint NOT NULL DEFAULT 0
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		t.Fatal(err)
	}
	return raw, searchDSN(base, schema)
}
