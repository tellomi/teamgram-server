package push

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUnregisterSessionDropsOnlyThatAuthKey(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	for _, d := range []Device{
		{AuthKeyID: 10, UserID: 9, TokenType: TokenAPNS, Token: "live", UpdatedAt: now},
		{AuthKeyID: 10, UserID: 9, TokenType: TokenAPNSVoIP, Token: "live-voip", UpdatedAt: now},
		{AuthKeyID: 11, UserID: 9, TokenType: TokenAPNS, Token: "other-session", UpdatedAt: now},
		{AuthKeyID: 10, UserID: 8, TokenType: TokenAPNS, Token: "other-user", UpdatedAt: now},
	} {
		if err := store.Upsert(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := UnregisterSession(ctx, store, 9, 10); err != nil {
		t.Fatal(err)
	}
	left, err := store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Token != "other-session" {
		t.Fatalf("user 9 devices %+v", left)
	}
	other, err := store.ListByUser(ctx, 8)
	if err != nil || len(other) != 1 || other[0].Token != "other-user" {
		t.Fatalf("user 8 devices %+v %v", other, err)
	}
	if err := UnregisterSession(ctx, store, 9, 10); err != nil {
		t.Fatal(err)
	}
	left, _ = store.ListByUser(ctx, 9)
	if len(left) != 1 {
		t.Fatalf("second unregister changed %+v", left)
	}
}

func TestUnregisterSessionZeroDropsEveryDeviceOfTheUser(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenAPNS, Token: "a"})
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenFCM, Token: "b"})
	_ = store.Upsert(ctx, Device{AuthKeyID: 3, UserID: 4, TokenType: TokenAPNS, Token: "c"})
	if err := UnregisterSession(ctx, store, 9, 0); err != nil {
		t.Fatal(err)
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 0 {
		t.Fatalf("user 9 still has %+v", left)
	}
	other, _ := store.ListByUser(ctx, 4)
	if len(other) != 1 {
		t.Fatalf("user 4 %+v", other)
	}
	if err := UnregisterSession(ctx, nil, 9, 0); err != nil {
		t.Fatal(err)
	}
}

func TestUnregisterSessionReturnsFirstErrorAndKeepsGoing(t *testing.T) {
	store := &failAfter{MemoryStore: &MemoryStore{}, failKey: 2}
	ctx := context.Background()
	_ = store.Upsert(ctx, Device{AuthKeyID: 1, UserID: 9, TokenType: TokenAPNS, Token: "a"})
	_ = store.Upsert(ctx, Device{AuthKeyID: 2, UserID: 9, TokenType: TokenAPNS, Token: "b"})
	_ = store.Upsert(ctx, Device{AuthKeyID: 3, UserID: 9, TokenType: TokenAPNS, Token: "c"})
	err := UnregisterSession(ctx, store, 9, 2, 1, 3)
	if err == nil || !strings.Contains(err.Error(), "fail 2") {
		t.Fatal(err)
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 0 {
		t.Fatalf("stopped early, left %+v", left)
	}
}

type failAfter struct {
	*MemoryStore
	failKey int64
}

func (f *failAfter) DeleteAuthKey(ctx context.Context, userID, authKeyID int64) error {
	if authKeyID == f.failKey {
		_ = f.MemoryStore.DeleteAuthKey(ctx, userID, authKeyID)
		return errors.New("fail 2")
	}
	return f.MemoryStore.DeleteAuthKey(ctx, userID, authKeyID)
}

func TestRevokedAuthKeysSkipsCaller(t *testing.T) {
	got := RevokedAuthKeys([]int64{100, 101}, []int64{0, 100, 200, 200, 101, 300})
	want := []int64{200, 300}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
	if RevokedAuthKeys(nil, nil) != nil {
		t.Fatal("expected no keys")
	}
}

func TestListQuerySkipsLoggedOutSessions(t *testing.T) {
	q := strings.ToLower(listByUserQuery)
	for _, part := range []string{
		"join auth_users au on au.auth_key_id = d.auth_key_id and au.user_id = d.user_id and au.deleted = 0",
		"left join auths a",
		"order by d.updated_at desc, d.id desc",
	} {
		if !strings.Contains(q, part) {
			t.Fatalf("list query missing %q\n%s", part, listByUserQuery)
		}
	}
	if strings.Contains(q, "left join auth_users") {
		t.Fatal("logged-out auth_users rows would still be pushed")
	}
	where := strings.ToLower(staleDeviceWhere)
	for _, part := range []string{"auth_users", "au.auth_key_id = d.auth_key_id", "au.user_id = d.user_id", "au.deleted = 0", "not exists"} {
		if !strings.Contains(where, part) {
			t.Fatalf("cleanup predicate missing %q\n%s", part, staleDeviceWhere)
		}
	}
}
