package password

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestOpenStoreDoesNotConfirmPhone(t *testing.T) {
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	ctx := context.Background()

	blank := openStore("")
	if _, err := blank.(identityReader).IdentityEmail(ctx, 7); !errors.Is(err, ErrIdentityUnconfirmed) {
		t.Fatalf("empty DSN: %v", err)
	}

	prev := openDatabase
	openDatabase = func(string, string) (*sql.DB, error) {
		return nil, errors.New("driver refused")
	}
	t.Cleanup(func() { openDatabase = prev })
	opened := openStore("user:pass@tcp(127.0.0.1:3306)/teamgram")
	if _, err := opened.(identityReader).IdentityEmail(ctx, 7); !errors.Is(err, ErrIdentityUnconfirmed) {
		t.Fatalf("sql.Open: %v", err)
	}
	openDatabase = prev

	ping := openStore("user:pass@tcp(127.0.0.1:1)/teamgram?timeout=200ms")
	if _, err := ping.(identityReader).IdentityEmail(ctx, 7); !errors.Is(err, ErrIdentityUnconfirmed) {
		t.Fatalf("ping: %v", err)
	}

	dead := New(Options{Store: blank})
	previous := Swap(dead)
	t.Cleanup(func() { Swap(previous) })
	pw, err := AccountPassword(ctx, 7, 1)
	if pw != nil || statusText(err) != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("getPassword on a dead store: %#v %v", pw, err)
	}
}

func TestIdentityValuesStayDistinct(t *testing.T) {
	svc := New(Options{Store: newMemoryStore()})
	prev := Swap(svc)
	t.Cleanup(func() { Swap(prev) })
	ctx := context.Background()

	if _, err := IdentityEmail(ctx, 7); !errors.Is(err, ErrIdentityUnconfirmed) {
		t.Fatalf("no row: %v", err)
	}
	SeedIdentity(7, "not-an-email")
	if _, err := IdentityEmail(ctx, 7); !errors.Is(err, ErrIdentityUnconfirmed) {
		t.Fatalf("dirty: %v", err)
	}
	SeedPhone(8)
	email, err := IdentityEmail(ctx, 8)
	if err != nil || email != "" {
		t.Fatalf("phone: %q %v", email, err)
	}
	SeedIdentity(9, "User@Tellomi.app")
	email, err = IdentityEmail(ctx, 9)
	if err != nil || email != "user@tellomi.app" {
		t.Fatalf("identity: %q %v", email, err)
	}
}
