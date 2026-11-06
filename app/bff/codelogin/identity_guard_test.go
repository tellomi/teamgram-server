package codelogin

import (
	"context"
	"strings"
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

func TestDivergentRedisIsNotMailedBeforeSendCode(t *testing.T) {
	svc, phones, emails, _, _ := harness(t)
	ctx := context.Background()
	// Plant the old copy before the identity guard exists, the way a row
	// written by an earlier build would still be in Redis.
	if err := svc.SaveLoginEmail(ctx, 7, "other@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveLoginEmail(ctx, 9, "user@tellomi.app"); err != nil {
		t.Fatal(err)
	}
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

	_, err := svc.RequestPasswordRecovery(ctx, 7)
	if rpcText(err) != "PASSWORD_RECOVERY_NA" {
		t.Fatal(err)
	}
	if len(*emails) != 0 {
		t.Fatalf("recovery mailed %v", *emails)
	}

	putPhone(phones, 8, "8613800138000", "h", "123456", 9)
	_, err = svc.ResetLoginEmail(ctx, 8, "8613800138000", "h", 9)
	if rpcText(err) != "EMAIL_INSTALL_MISSING" {
		t.Fatal(err)
	}
	if len(*emails) != 0 {
		t.Fatalf("reset mailed %v", *emails)
	}

	putPhone(phones, 2, "user@tellomi.app", "h2", "old", 7)
	sent, err := svc.ResetLoginEmail(ctx, 2, "user@tellomi.app", "h2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(*emails) != 1 || (*emails)[0].to != "user@tellomi.app" {
		t.Fatalf("email reset %#v", *emails)
	}
	if sent.GetType().GetEmailPattern() != "u***@tellomi.app" {
		t.Fatal(sent.GetType().GetEmailPattern())
	}
	got, _ := svc.LoginEmail(ctx, 7)
	if got != "other@tellomi.app" {
		t.Fatalf("redis rewritten to %q", got)
	}
}

func TestEmailIdentityRecoveryDoesNotUseTheLoginCopy(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	prev := password.Swap(password.New(password.Options{}))
	t.Cleanup(func() { password.Swap(prev) })
	password.SeedIdentity(7, "user@tellomi.app")
	if err := password.SeedRecovery(7, "cloud@tellomi.app", true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveLoginEmail(ctx, 7, "other@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	svc.opt.IdentityEmail = password.IdentityEmail
	svc.opt.IdentityOwner = password.IdentityOwner

	got, err := svc.RequestPasswordRecovery(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(*emails) != 1 || (*emails)[0].to != "cloud@tellomi.app" {
		t.Fatalf("%#v", *emails)
	}
	if got.GetEmailPattern() != "c***@tellomi.app" || strings.Contains(got.GetEmailPattern(), "cloud") {
		t.Fatal(got.GetEmailPattern())
	}

	if err := password.SeedRecovery(7, "", false); err != nil {
		t.Fatal(err)
	}
	svc2, _, emails2, _, _ := harness(t)
	if err := svc2.SaveLoginEmail(ctx, 7, "user@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	svc2.opt.IdentityEmail = password.IdentityEmail
	svc2.opt.IdentityOwner = password.IdentityOwner
	_, err = svc2.RequestPasswordRecovery(ctx, 7)
	if rpcText(err) != "PASSWORD_RECOVERY_NA" {
		t.Fatal(err)
	}
	if len(*emails2) != 0 {
		t.Fatal(*emails2)
	}
}

func TestRecoveryEmailStaysOffTheLoginIdentity(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	prev := password.Swap(password.New(password.Options{}))
	t.Cleanup(func() { password.Swap(prev) })
	password.SeedIdentity(7, "user@tellomi.app")
	if err := password.SeedRecovery(7, "cloud@tellomi.app", true); err != nil {
		t.Fatal(err)
	}
	svc.opt.IdentityEmail = password.IdentityEmail
	svc.opt.IdentityOwner = password.IdentityOwner
	if err := svc.SaveLoginEmail(ctx, 7, "user@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RequestPasswordRecovery(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if (*emails)[0].to != "cloud@tellomi.app" {
		t.Fatalf("recovery destination %q", (*emails)[0].to)
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
	if recovery != "recover@tellomi.app" {
		t.Fatalf("recovery %q", recovery)
	}
}
