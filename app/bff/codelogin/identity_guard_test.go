package codelogin

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withIdentity(t *testing.T, svc *Service, emailOf func(int64) string, ownerOf func(string) int64) {
	t.Helper()
	svc.opt.IdentityEmail = func(_ context.Context, userID int64) (string, error) {
		if emailOf == nil {
			return "", nil
		}
		return emailOf(userID), nil
	}
	svc.opt.IdentityOwner = func(_ context.Context, email string) (int64, error) {
		if ownerOf == nil {
			return 0, nil
		}
		return ownerOf(email), nil
	}
}

func TestEmailIdentityChangeDoesNotReplaceLogin(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	withIdentity(t, svc, func(id int64) string {
		if id == 7 {
			return "user@tellomi.app"
		}
		return ""
	}, func(email string) int64 {
		if email == "user@tellomi.app" {
			return 7
		}
		return 0
	})
	if err := svc.SaveLoginEmail(ctx, 7, "user@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SendVerifyEmailCode(ctx, 4, 7, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: changePurpose(),
		Email:   "other@tellomi.app",
	})
	if rpcText(err) != "EMAIL_NOT_ALLOWED" {
		t.Fatal(err)
	}
	if len(*emails) != 0 {
		t.Fatalf("sent %d", len(*emails))
	}
	got, _ := svc.LoginEmail(ctx, 7)
	if got != "user@tellomi.app" {
		t.Fatal(got)
	}
	if _, err = svc.SendVerifyEmailCode(ctx, 4, 7, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: changePurpose(),
		Email:   "user@tellomi.app",
	}); err != nil {
		t.Fatal(err)
	}
	if len(*emails) != 1 || (*emails)[0].to != "user@tellomi.app" {
		t.Fatalf("%#v", *emails)
	}
}

func TestLoginEmailCannotTakeAnotherIdentity(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	withIdentity(t, svc, func(int64) string { return "" }, func(email string) int64 {
		if email == "user@tellomi.app" {
			return 7
		}
		return 0
	})
	if err := svc.SaveLoginEmail(ctx, 9, "mine@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SendVerifyEmailCode(ctx, 4, 9, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: changePurpose(),
		Email:   "user@tellomi.app",
	})
	if rpcText(err) != "EMAIL_NOT_ALLOWED" {
		t.Fatal(err)
	}
	if len(*emails) != 0 {
		t.Fatal(*emails)
	}
	got, _ := svc.LoginEmail(ctx, 9)
	if got != "mine@tellomi.app" {
		t.Fatal(got)
	}
}

func TestEmailSetupCannotAttachASecondAddress(t *testing.T) {
	svc, phones, emails, _, _ := harness(t)
	ctx := context.Background()
	putPhone(phones, 2, "user@tellomi.app", "h", "old", 7)
	_, err := svc.SendVerifyEmailCode(ctx, 2, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: setupPurpose("user@tellomi.app", "h"),
		Email:   "other@tellomi.app",
	})
	if rpcText(err) != "EMAIL_NOT_ALLOWED" {
		t.Fatal(err)
	}
	if len(*emails) != 0 {
		t.Fatal(*emails)
	}
}

func TestRecoveryEmailStaysOffTheLoginIdentity(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	prev := password.Swap(password.New(password.Options{}))
	t.Cleanup(func() { password.Swap(prev) })
	password.SeedIdentity(7, "user@tellomi.app")
	svc.opt.IdentityEmail = password.IdentityEmail
	svc.opt.IdentityOwner = password.IdentityOwner
	if err := svc.SaveLoginEmail(ctx, 7, "user@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RequestPasswordRecovery(ctx, 7); err != nil {
		t.Fatal(err)
	}
	code := (*emails)[0].code
	settings := &mtproto.Account_PasswordInputSettings{
		Email: &wrapperspb.StringValue{Value: "recover@tellomi.app"},
	}
	if err := svc.RecoverPassword(ctx, 7, code, settings); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.LoginEmail(ctx, 7)
	if got != "user@tellomi.app" {
		t.Fatalf("login identity redis %q", got)
	}
	recovery, _, err := password.RecoveryEmail(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if recovery == "user@tellomi.app" {
		t.Fatal("login identity was copied into the recovery address")
	}
}
