package channel

import (
	"context"
	"testing"
)

func TestIsBroadcastUsesTheChannelRecord(t *testing.T) {
	svc := New(NewMemoryStore())
	ctx := context.Background()
	mega, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	megaID := mega.Chats[0].GetId()
	feedID := feed.Chats[0].GetId()

	got, ok, err := svc.IsBroadcast(ctx, megaID)
	if err != nil || !ok || got {
		t.Fatalf("megagroup broadcast=%v ok=%v err=%v", got, ok, err)
	}
	got, ok, err = svc.IsBroadcast(ctx, feedID)
	if err != nil || !ok || !got {
		t.Fatalf("broadcast flag=%v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err = svc.IsBroadcast(ctx, megaID+feedID+99); err != nil || ok {
		t.Fatalf("missing ok=%v err=%v", ok, err)
	}
}
