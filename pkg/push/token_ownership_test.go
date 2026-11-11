package push

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegistrationDropsUndeclaredTokenOwner(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	old := Device{UserID: 9, AuthKeyID: 90, TokenType: TokenFCM, Token: "fixture-shared-device"}
	newer := Device{UserID: 10, AuthKeyID: 100, TokenType: TokenFCM, Token: old.Token}
	if err := store.Upsert(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(ctx, newer); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListByUser(ctx, 9)
	if len(list) != 0 {
		t.Fatal("registration retained undeclared previous account on shared token")
	}
	fcm := &captureFCM{}
	svc := NewService(store, Config{})
	svc.SetFCM(fcm)
	svc.Deliver(ctx, 9, textUpdate())
	if len(fcm.body) != 0 {
		t.Fatal("message for previous account reached transferred token")
	}
	svc.Deliver(ctx, 10, textUpdate())
	var payload map[string]string
	if err := json.Unmarshal(fcm.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["user_id"] != "10" {
		t.Fatal("new account routing was lost")
	}
}

func TestRegistrationPreservesOnlyDeclaredCoAccounts(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	for _, d := range []Device{
		{UserID: 9, AuthKeyID: 90, TokenType: TokenAPNS, Token: "fixture-shared"},
		{UserID: 11, AuthKeyID: 110, TokenType: TokenAPNS, Token: "fixture-shared"},
		{UserID: 12, AuthKeyID: 120, TokenType: TokenAPNSVoIP, Token: "fixture-shared"},
		{UserID: 13, AuthKeyID: 130, TokenType: TokenAPNS, Token: "fixture-other"},
	} {
		if err := store.Upsert(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	// Explicitly repopulate both co-accounts; the latest declaration is authoritative.
	_ = store.Upsert(ctx, Device{UserID: 9, AuthKeyID: 90, TokenType: TokenAPNS, Token: "fixture-shared", OtherUIDs: []int64{11}})
	_ = store.Upsert(ctx, Device{UserID: 10, AuthKeyID: 100, TokenType: TokenAPNS, Token: "fixture-shared", OtherUIDs: []int64{9, 9, 10, 0, -1}})
	for uid, want := range map[int64]int{9: 1, 10: 1, 11: 0, 12: 1, 13: 1} {
		list, _ := store.ListByUser(ctx, uid)
		if len(list) != want {
			t.Fatalf("user %d registrations=%d want=%d", uid, len(list), want)
		}
	}
}
