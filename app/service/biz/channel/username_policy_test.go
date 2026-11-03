package channel

import (
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"

	"google.golang.org/grpc/status"
)

func TestChannelUsernamePolicy(t *testing.T) {
	t.Setenv(usernamepolicy.EnvEnabled, "1")
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 7, Title: "Old", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())

	if _, err := svc.CheckUsername(ctx, 7, in, "admin"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("admin: %v", err)
	}
	if _, err := svc.CheckUsername(ctx, 7, in, "admin01"); err != nil {
		t.Fatalf("admin01: %v", err)
	}
	if _, err := svc.CheckUsername(ctx, 7, in, "qq"); status.Convert(err).Message() != "USERNAME_INVALID" {
		t.Fatalf("qq: %v", err)
	}
	if _, err := svc.UpdateUsername(ctx, 7, 0, in, "google"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("google: %v", err)
	}
	if _, err := svc.UpdateUsername(ctx, 7, 0, in, "roomname"); err != nil {
		t.Fatalf("roomname: %v", err)
	}
	if _, err := svc.UpdateUsername(ctx, 7, 0, in, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
}
