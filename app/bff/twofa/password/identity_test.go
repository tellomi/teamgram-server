package password

import (
	"context"
	"strings"
	"testing"
)

func TestIdentitySQLIsReadOnly(t *testing.T) {
	for _, q := range []string{identityEmailSQL, identityOwnerSQL} {
		lower := strings.ToLower(strings.TrimSpace(q))
		if !strings.HasPrefix(lower, "select") {
			t.Fatal(q)
		}
		for _, verb := range []string{"update ", "insert ", "delete ", "alter "} {
			if strings.Contains(lower, verb) {
				t.Fatal(q)
			}
		}
	}
}

func TestLoginEmailPatternFromIdentity(t *testing.T) {
	svc := New(Options{})
	prev := Swap(svc)
	t.Cleanup(func() { Swap(prev) })
	SeedIdentity(7, "User@Tellomi.app")

	pw, err := AccountPassword(context.Background(), 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetLoginEmailPattern().GetValue() != "u***@tellomi.app" {
		t.Fatalf("pattern %q", pw.GetLoginEmailPattern().GetValue())
	}
	if pw.GetHasPassword() || pw.GetHasRecovery() || pw.GetEmailUnconfirmedPattern() != nil {
		t.Fatalf("recovery must stay independent: %+v", pw)
	}

	phone, err := AccountPassword(context.Background(), 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	if phone.GetLoginEmailPattern() != nil {
		t.Fatalf("phone account pattern %q", phone.GetLoginEmailPattern().GetValue())
	}
}

func TestLoginEmailPatternDoesNotReplaceRecoveryEmail(t *testing.T) {
	svc := New(Options{})
	prev := Swap(svc)
	t.Cleanup(func() { Swap(prev) })
	SeedIdentity(7, "user@tellomi.app")
	if err := svc.store.Save(context.Background(), Record{
		UserID:           7,
		EmailUnconfirmed: "recover@tellomi.app",
		Exists:           true,
	}); err != nil {
		t.Fatal(err)
	}
	pw, err := AccountPassword(context.Background(), 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetLoginEmailPattern().GetValue() != "u***@tellomi.app" {
		t.Fatal(pw.GetLoginEmailPattern())
	}
	if pw.GetEmailUnconfirmedPattern().GetValue() != "r***@tellomi.app" {
		t.Fatal(pw.GetEmailUnconfirmedPattern())
	}
	if pw.GetHasRecovery() {
		t.Fatal("unconfirmed recovery email is not has_recovery")
	}
	got, err := IdentityEmail(context.Background(), 7)
	if err != nil || got != "user@tellomi.app" {
		t.Fatalf("%q %v", got, err)
	}
}
