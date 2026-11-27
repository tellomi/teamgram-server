package channel

import (
	"context"
	"os"
	"testing"
)

// TestMySQLStaleFeatureSaveKeepsBothJoinTops uses a real MySQL connection
// when TEAMGRAM_MYSQL_DSN or the local teamgram DSN answers. It is not an
// MTProto session. A refused connection skips the test.
func TestMySQLStaleFeatureSaveKeepsBothJoinTops(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
	}
	svc, err := OpenMySQL(dsn)
	if err != nil {
		t.Skipf("mysql not available, stale feature merge was not run on a database: %v", err)
	}
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 910188901, Title: "mysql-join-merge", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	chID := created.Chats[0].Id
	if _, err = svc.TogglePreHistory(ctx, 910188901, 0, inCh(created.Chats[0]), true); err != nil {
		t.Fatal(err)
	}
	first, err := svc.store.GetFeatures(ctx, chID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.store.GetFeatures(ctx, chID)
	if err != nil {
		t.Fatal(err)
	}
	if first.JoinTop == nil {
		first.JoinTop = map[int64]int32{}
	}
	if second.JoinTop == nil {
		second.JoinTop = map[int64]int32{}
	}
	first.JoinTop[21] = 2
	second.JoinTop[22] = 2
	second.SlowmodeSeconds = 15
	if err = svc.store.SaveFeatures(ctx, chID, second); err != nil {
		t.Fatal(err)
	}
	if err = svc.store.SaveFeatures(ctx, chID, first); err != nil {
		t.Fatal(err)
	}
	got, err := svc.mutableFeatures(ctx, chID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HiddenPrehistory || got.SlowmodeSeconds != 15 || got.JoinTop[21] != 2 || got.JoinTop[22] != 2 {
		t.Fatalf("mysql merge %+v", got)
	}
}
