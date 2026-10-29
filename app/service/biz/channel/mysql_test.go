package channel

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMySQLRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/teamgram?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
	}
	svc, err := OpenMySQL(dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 9001, Title: "mysql-channel", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	sent, err := svc.Send(ctx, 9001, ch.Id, 0, []Outgoing{{
		RandomID: 4242,
		Message:  &mtproto.Message{Message: "persisted", Date: 30},
	}})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	hist, err := reopened.History(ctx, 9001, in, HistoryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if hist.Pts < sent.Updates[0].GetPts_INT32() || hist.Count < 2 {
		t.Fatalf("reopened history pts=%d count=%d", hist.Pts, hist.Count)
	}
	diff, err := reopened.Difference(ctx, 9001, in, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Pts != hist.Pts || len(diff.NewMessages) < 2 {
		t.Fatalf("difference pts=%d messages=%d", diff.Pts, len(diff.NewMessages))
	}
}
