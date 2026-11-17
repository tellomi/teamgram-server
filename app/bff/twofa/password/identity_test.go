package password

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/status"
)

func statusText(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	return err.Error()
}

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
	pattern := pw.GetLoginEmailPattern().GetValue()
	if pattern != "u***@tellomi.app" || strings.Contains(pattern, "user") || strings.Contains(pattern, "User") {
		t.Fatalf("pattern %q", pattern)
	}
	if pw.GetHasPassword() || pw.GetHasRecovery() || pw.GetEmailUnconfirmedPattern() != nil {
		t.Fatalf("recovery must stay independent: %+v", pw)
	}

	unconfirmed, err := AccountPassword(context.Background(), 8, 1)
	if unconfirmed != nil || statusText(err) != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("unknown identity must fail closed: %#v %v", unconfirmed, err)
	}
	SeedPhone(8)
	phone, err := AccountPassword(context.Background(), 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	if phone.GetLoginEmailPattern() != nil {
		t.Fatalf("phone account pattern %q", phone.GetLoginEmailPattern().GetValue())
	}
}

func TestLoginEmailPatternShowsAttachedPhoneEmail(t *testing.T) {
	svc := New(Options{})
	prev := Swap(svc)
	t.Cleanup(func() {
		Swap(prev)
		SetAttachedLoginEmail(nil)
	})
	SeedPhone(8)
	SetAttachedLoginEmail(func(_ context.Context, userID int64) (string, error) {
		if userID == 8 {
			return "phone@tellomi.app", nil
		}
		return "other@tellomi.app", nil
	})
	phone, err := AccountPassword(context.Background(), 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	if phone.GetLoginEmailPattern().GetValue() != "p***@tellomi.app" {
		t.Fatalf("pattern %q", phone.GetLoginEmailPattern().GetValue())
	}

	SeedIdentity(7, "user@tellomi.app")
	emailAccount, err := AccountPassword(context.Background(), 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if emailAccount.GetLoginEmailPattern().GetValue() != "u***@tellomi.app" {
		t.Fatalf("identity must win: %q", emailAccount.GetLoginEmailPattern().GetValue())
	}

	SetAttachedLoginEmail(func(context.Context, int64) (string, error) {
		return "", errors.New("redis down")
	})
	failed, err := AccountPassword(context.Background(), 8, 1)
	if failed != nil || statusText(err) != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("%#v %v", failed, err)
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
