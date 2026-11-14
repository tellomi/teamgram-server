package channel

import (
	"testing"

	"google.golang.org/grpc/status"
)

func TestRenameReleasesPreviousUsername(t *testing.T) {
	svc, ctx := newTest(t)
	first, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "One", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(ctx, CreateRequest{CreatorID: 2, Title: "Two", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	in1 := input(first.Chats[0].Id, first.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	in2 := input(second.Chats[0].Id, second.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 1, 0, in1, "oldname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, in1, "newname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveUsername(ctx, 9, "oldname"); status.Convert(err).Message() != "USERNAME_NOT_OCCUPIED" {
		t.Fatalf("old username still resolves: %v", err)
	}
	if _, err := svc.CheckUsername(ctx, 2, in2, "oldname"); err != nil {
		t.Fatalf("old username still occupied: %v", err)
	}
	if _, err := svc.UpdateUsername(ctx, 2, 0, in2, "oldname"); err != nil {
		t.Fatalf("other channel could not take the released name: %v", err)
	}
	got, err := svc.ResolveUsername(ctx, 9, "oldname")
	if err != nil || got.GetId() != second.Chats[0].Id {
		t.Fatalf("old name resolves to %#v %v", got, err)
	}
	got, err = svc.ResolveUsername(ctx, 9, "newname")
	if err != nil || got.GetId() != first.Chats[0].Id {
		t.Fatalf("new name resolves to %#v %v", got, err)
	}
}
