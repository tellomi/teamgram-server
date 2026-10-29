package story

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
	now := int32(1_700_000_000)
	svc.SetClock(func() int32 { return now })
	owner := Owner{Kind: KindUser, ID: 88001}
	up, err := svc.Send(ctx, SendRequest{
		Actor: owner.ID, Owner: owner, Media: photoMedia(), Rules: rules("all"), RandomID: 88001, Caption: "mysql",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	reopened, err := OpenMySQL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetClock(func() int32 { return now })
	got, err := reopened.GetPeerStories(ctx, owner.ID, owner, 0)
	if err != nil || len(got.Stories.Stories) != 1 || got.Stories.Stories[0].GetId() != id {
		t.Fatalf("reopened %+v %v", got, err)
	}
	if got.Stories.Stories[0].GetCaption().GetValue() != "mysql" {
		t.Fatalf("caption %q", got.Stories.Stories[0].GetCaption().GetValue())
	}
	_ = mtproto.Layer
}
