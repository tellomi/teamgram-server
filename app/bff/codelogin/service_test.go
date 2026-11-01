package codelogin

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/pkg/srp"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func rpcText(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok && st.Message() != "" {
		return st.Message()
	}
	return err.Error()
}

type sent struct {
	to   string
	code string
}

func harness(t *testing.T) (*Service, *MemPhones, *[]sent, *[]sent, *time.Time) {
	t.Helper()
	now := time.Unix(1_700_000_000, 0)
	phones := NewMemPhones()
	var emails, smss []sent
	svc := New(nil, Options{
		Now:         func() time.Time { return now },
		EmailLen:    6,
		SMSLen:      6,
		TTL:         180 * time.Second,
		MaxAttempts: 5,
		Phones:      phones,
		SendEmail: func(_ context.Context, to, code string) (string, error) {
			emails = append(emails, sent{to, code})
			return code, nil
		},
		SendSMS: func(_ context.Context, phone, code string) (string, error) {
			smss = append(smss, sent{phone, code})
			return code, nil
		},
		LookupUser: func(context.Context, string) (int64, error) { return 42, nil },
	})
	return svc, phones, &emails, &smss, &now
}

func setupPurpose(phone, hash string) *mtproto.EmailVerifyPurpose {
	return mtproto.MakeTLEmailVerifyPurposeLoginSetup(&mtproto.EmailVerifyPurpose{
		PhoneNumber:   phone,
		PhoneCodeHash: hash,
	}).To_EmailVerifyPurpose()
}

func changePurpose() *mtproto.EmailVerifyPurpose {
	return mtproto.MakeTLEmailVerifyPurposeLoginChange(&mtproto.EmailVerifyPurpose{}).To_EmailVerifyPurpose()
}

func codeVer(code string) *mtproto.EmailVerification {
	return mtproto.MakeTLEmailVerificationCode(&mtproto.EmailVerification{Code: code}).To_EmailVerification()
}

func putPhone(phones *MemPhones, authKey int64, phone, hash, code string, user int64) {
	phones.Put(&PhoneCode{
		AuthKeyID:  authKey,
		Phone:      phone,
		Hash:       hash,
		Code:       code,
		Extra:      code,
		Expires:    time.Unix(1_700_000_000, 0).Add(time.Hour).Unix(),
		SentType:   model.SentCodeTypeSms,
		UserID:     user,
		Registered: user != 0,
	})
}

func TestSendVerifyEmailCodeAndVerifyLoginSetup(t *testing.T) {
	svc, phones, emails, smss, _ := harness(t)
	putPhone(phones, 7, "8613800138000", "hash-1", "000000", 42)
	sentCode, err := svc.SendVerifyEmailCode(context.Background(), 7, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: setupPurpose("8613800138000", "hash-1"),
		Email:   "User@Tellomi.app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sentCode.GetEmailPattern() != "u***@tellomi.app" || sentCode.GetLength() != 6 {
		t.Fatalf("sent %#v", sentCode)
	}
	if len(*emails) != 1 || (*emails)[0].to != "user@tellomi.app" {
		t.Fatalf("email delivery %#v", *emails)
	}
	verified, err := svc.VerifyEmail(context.Background(), 7, 0, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      setupPurpose("+86 138 0013 8000", "hash-1"),
		Verification: codeVer((*emails)[0].code),
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.GetPredicateName() != mtproto.Predicate_account_emailVerifiedLogin {
		t.Fatalf("predicate %s", verified.GetPredicateName())
	}
	if verified.GetSentCode().GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeSms {
		t.Fatalf("next code %#v", verified.GetSentCode())
	}
	if len(*smss) != 1 || (*smss)[0].to != "8613800138000" {
		t.Fatalf("sms %#v", *smss)
	}
	got, _ := svc.LoginEmail(context.Background(), 42)
	if got != "user@tellomi.app" {
		t.Fatalf("saved email %q", got)
	}
	pc, _ := phones.Get(context.Background(), 7, "8613800138000", "hash-1")
	if pc == nil || pc.Code != (*smss)[0].code || pc.SentType != model.SentCodeTypeSms {
		t.Fatalf("phone code %#v", pc)
	}
}

func TestSendVerifyEmailCodeErrors(t *testing.T) {
	svc, phones, _, _, _ := harness(t)
	ctx := context.Background()
	_, err := svc.SendVerifyEmailCode(ctx, 1, 0, &mtproto.TLAccountSendVerifyEmailCode{Email: "not-an-email", Purpose: setupPurpose("8613800138000", "h")})
	if rpcText(err) != "EMAIL_INVALID" {
		t.Fatal(err)
	}
	_, err = svc.SendVerifyEmailCode(ctx, 1, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Email:   "a@tellomi.app",
		Purpose: mtproto.MakeTLEmailVerifyPurposePassport(&mtproto.EmailVerifyPurpose{}).To_EmailVerifyPurpose(),
	})
	if rpcText(err) != "EMAIL_NOT_ALLOWED" {
		t.Fatal(err)
	}
	_, err = svc.SendVerifyEmailCode(ctx, 1, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Email:   "a@tellomi.app",
		Purpose: setupPurpose("8613800138000", "missing"),
	})
	if rpcText(err) != "PHONE_HASH_EXPIRED" {
		t.Fatal(err)
	}
	_, err = svc.SendVerifyEmailCode(ctx, 1, 9, &mtproto.TLAccountSendVerifyEmailCode{
		Email:   "a@tellomi.app",
		Purpose: changePurpose(),
	})
	if rpcText(err) != "EMAIL_NOT_SETUP" {
		t.Fatal(err)
	}
	_ = phones
}

func TestVerifyEmailCodeInvalidAndTokens(t *testing.T) {
	svc, phones, emails, _, _ := harness(t)
	ctx := context.Background()
	putPhone(phones, 3, "8613800138000", "h", "111111", 0)
	if _, err := svc.SendVerifyEmailCode(ctx, 3, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: setupPurpose("8613800138000", "h"),
		Email:   "a@tellomi.app",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.VerifyEmail(ctx, 3, 0, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      setupPurpose("8613800138000", "h"),
		Verification: codeVer("000000"),
	})
	if rpcText(err) != "CODE_INVALID" {
		t.Fatal(err)
	}
	_, err = svc.VerifyEmail(ctx, 3, 0, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      setupPurpose("8613800138000", "h"),
		Verification: mtproto.MakeTLEmailVerificationGoogle(&mtproto.EmailVerification{Token: "tok"}).To_EmailVerification(),
	})
	if rpcText(err) != "EMAIL_INVALID" {
		t.Fatal(err)
	}
	_, err = svc.VerifyEmail(ctx, 3, 0, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      setupPurpose("8613800138000", "h"),
		Verification: mtproto.MakeTLEmailVerificationApple(&mtproto.EmailVerification{Token: "tok"}).To_EmailVerification(),
	})
	if rpcText(err) != "EMAIL_INVALID" {
		t.Fatal(err)
	}
	if len(*emails) != 1 {
		t.Fatalf("should not resend on a bad code, got %d", len(*emails))
	}
}

func TestVerifyEmailExpired(t *testing.T) {
	svc, phones, emails, _, now := harness(t)
	ctx := context.Background()
	putPhone(phones, 3, "8613800138000", "h", "111111", 0)
	if _, err := svc.SendVerifyEmailCode(ctx, 3, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: setupPurpose("8613800138000", "h"),
		Email:   "a@tellomi.app",
	}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Hour)
	_, err := svc.VerifyEmail(ctx, 3, 0, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      setupPurpose("8613800138000", "h"),
		Verification: codeVer((*emails)[0].code),
	})
	if rpcText(err) != "EMAIL_VERIFY_EXPIRED" {
		t.Fatal(err)
	}
}

func TestVerifyEmailLoginChange(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	if err := svc.SaveLoginEmail(ctx, 9, "old@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendVerifyEmailCode(ctx, 4, 9, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: changePurpose(),
		Email:   "new@tellomi.app",
	}); err != nil {
		t.Fatal(err)
	}
	verified, err := svc.VerifyEmail(ctx, 4, 9, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose:      changePurpose(),
		Verification: codeVer((*emails)[0].code),
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.GetPredicateName() != mtproto.Predicate_account_emailVerified || verified.GetEmail() != "new@tellomi.app" {
		t.Fatalf("%#v", verified)
	}
	got, _ := svc.LoginEmail(ctx, 9)
	if got != "new@tellomi.app" {
		t.Fatal(got)
	}
}

func TestPasswordRecovery(t *testing.T) {
	svc, _, emails, _, _ := harness(t)
	ctx := context.Background()
	_, err := svc.RequestPasswordRecovery(ctx, 9)
	if rpcText(err) != "PASSWORD_RECOVERY_NA" {
		t.Fatal(err)
	}
	if err = svc.SaveLoginEmail(ctx, 9, "recover@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.RequestPasswordRecovery(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetEmailPattern() != "r***@tellomi.app" || len(*emails) != 1 {
		t.Fatalf("%#v %#v", got, *emails)
	}
	if _, err = svc.CheckRecoveryPassword(ctx, 9, ""); rpcText(err) != "CODE_EMPTY" {
		t.Fatal(err)
	}
	if _, err = svc.CheckRecoveryPassword(ctx, 9, "000000"); rpcText(err) != "CODE_INVALID" {
		t.Fatal(err)
	}
	if _, err = svc.CheckRecoveryPassword(ctx, 9, (*emails)[0].code); err != nil {
		t.Fatal(err)
	}
	bad := &mtproto.Account_PasswordInputSettings{}
	if err = svc.RecoverPassword(ctx, 9, (*emails)[0].code, bad); rpcText(err) != "NEW_SETTINGS_INVALID" {
		t.Fatal(err)
	}
	if err = svc.RecoverPassword(ctx, 9, (*emails)[0].code, nil); err != nil {
		t.Fatal(err)
	}
	if err = svc.RecoverPassword(ctx, 9, (*emails)[0].code, nil); rpcText(err) != "PASSWORD_RECOVERY_EXPIRED" {
		t.Fatal(err)
	}
}

func TestPasswordRecoveryExpired(t *testing.T) {
	svc, _, emails, _, now := harness(t)
	ctx := context.Background()
	_ = svc.SaveLoginEmail(ctx, 3, "a@tellomi.app")
	if _, err := svc.RequestPasswordRecovery(ctx, 3); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Hour)
	_, err := svc.CheckRecoveryPassword(ctx, 3, (*emails)[0].code)
	if rpcText(err) != "PASSWORD_RECOVERY_EXPIRED" {
		t.Fatal(err)
	}
}

func TestResetLoginEmail(t *testing.T) {
	svc, phones, emails, _, _ := harness(t)
	ctx := context.Background()
	putPhone(phones, 8, "8613800138000", "h", "123456", 42)
	if err := svc.SaveLoginEmail(ctx, 42, "login@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.ResetLoginEmail(ctx, 8, "8613800138000", "h", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeEmailCode {
		t.Fatalf("%#v", sent.GetType())
	}
	if sent.GetType().GetEmailPattern() != "l***@tellomi.app" {
		t.Fatal(sent.GetType().GetEmailPattern())
	}
	if len(*emails) != 1 || (*emails)[0].to != "login@tellomi.app" {
		t.Fatalf("%#v", *emails)
	}
	pc, _ := phones.Get(ctx, 8, "8613800138000", "h")
	if pc.Code != (*emails)[0].code || pc.SentType != model.SentCodeTypeEmailCode {
		t.Fatalf("%#v", pc)
	}
	_, err = svc.ResetLoginEmail(ctx, 8, "8613800138000", "h", 42)
	if rpcText(err) != "TASK_ALREADY_EXISTS" {
		t.Fatal(err)
	}
	_, err = svc.ResetLoginEmail(ctx, 8, "8613800138000", "nope", 42)
	if rpcText(err) != "PHONE_CODE_EXPIRED" {
		t.Fatal(err)
	}
	_, err = svc.ResetLoginEmail(ctx, 1, "", "h", 0)
	if rpcText(err) != "PHONE_NUMBER_INVALID" {
		t.Fatal(err)
	}
}

func TestResetLoginEmailMissing(t *testing.T) {
	svc, phones, _, _, _ := harness(t)
	putPhone(phones, 1, "8613800138000", "h", "123456", 0)
	svc.opt.LookupUser = func(context.Context, string) (int64, error) { return 0, nil }
	_, err := svc.ResetLoginEmail(context.Background(), 1, "8613800138000", "h", 0)
	if rpcText(err) != "EMAIL_INSTALL_MISSING" {
		t.Fatal(err)
	}
}

func TestResetLoginEmailAddress(t *testing.T) {
	svc, phones, emails, _, _ := harness(t)
	putPhone(phones, 2, "user@tellomi.app", "h", "old", 5)
	sent, err := svc.ResetLoginEmail(context.Background(), 2, "user@tellomi.app", "h", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sent.GetPhoneCodeHash() != "h" || len(*emails) != 1 || (*emails)[0].to != "user@tellomi.app" {
		t.Fatalf("%#v %#v", sent, *emails)
	}
}

func TestRequestFirebaseSmsFallsBack(t *testing.T) {
	svc, phones, _, _, _ := harness(t)
	putPhone(phones, 1, "8613800138000", "h", "123456", 9)
	got, err := svc.RequestFirebaseSms(context.Background(), 1, "+86 138 0013 8000", "h")
	if err != nil || got != mtproto.BoolFalse {
		t.Fatalf("%v %v", got, err)
	}
	got, err = svc.RequestFirebaseSms(context.Background(), 1, "8613800138000", "missing")
	if err != nil || got != mtproto.BoolFalse {
		t.Fatalf("missing hash should still fall back: %v %v", got, err)
	}
	_, err = svc.RequestFirebaseSms(context.Background(), 1, "8613800138000", "")
	if rpcText(err) != "PHONE_CODE_EMPTY" {
		t.Fatal(err)
	}
	_, err = svc.RequestFirebaseSms(context.Background(), 1, "12", "h")
	if rpcText(err) != "PHONE_NUMBER_INVALID" {
		t.Fatal(err)
	}
}

func TestReportMissingCode(t *testing.T) {
	svc, _, _, _, _ := harness(t)
	var line string
	got, err := svc.ReportMissingCode("8613800138000", "hashval", "00", func(phone, hash, mnc string) {
		line = phone + " " + hash + " " + mnc
	})
	if err != nil || got != mtproto.BoolTrue || line != "8613800138000 hashval 00" {
		t.Fatalf("%v %v %q", got, err, line)
	}
	_, err = svc.ReportMissingCode("abc", "h", "", nil)
	if rpcText(err) != "PHONE_NUMBER_INVALID" {
		t.Fatal(err)
	}
}

func TestInvalidateSignInCodes(t *testing.T) {
	svc, phones, _, _, _ := harness(t)
	ctx := context.Background()
	putPhone(phones, 11, "8613800138000", "h", "123456", 9)
	if err := svc.RememberSignInCode(ctx, 9, 11, "8613800138000", "12-34-56", "h"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.InvalidateSignInCodes(ctx, 9, []string{"123456", "999999"})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatal(err)
	}
	pc, _ := phones.Get(ctx, 11, "8613800138000", "h")
	if pc != nil {
		t.Fatal("code should be deleted")
	}
	_, err = svc.InvalidateSignInCodes(ctx, 0, []string{"123456"})
	if rpcText(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
}

func TestConfirmPhone(t *testing.T) {
	svc, _, _, smss, _ := harness(t)
	ctx := context.Background()
	_, err := svc.SendConfirmPhoneCode(ctx, "no-such-hash")
	if rpcText(err) != "HASH_INVALID" {
		t.Fatal(err)
	}
	hash, err := svc.IssuePhoneConfirm(ctx, 4, "8613800138000")
	if err != nil || hash == "" {
		t.Fatal(err)
	}
	sent, err := svc.SendConfirmPhoneCode(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if sent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeSms || len(*smss) != 1 {
		t.Fatalf("%#v %#v", sent, *smss)
	}
	_, err = svc.ConfirmPhone(ctx, sent.GetPhoneCodeHash(), "")
	if rpcText(err) != "PHONE_CODE_EMPTY" {
		t.Fatal(err)
	}
	_, err = svc.ConfirmPhone(ctx, "bad", "123456")
	if rpcText(err) != "CODE_HASH_INVALID" {
		t.Fatal(err)
	}
	_, err = svc.ConfirmPhone(ctx, sent.GetPhoneCodeHash(), "000000")
	if rpcText(err) != "PHONE_CODE_INVALID" {
		t.Fatal(err)
	}
	got, err := svc.ConfirmPhone(ctx, sent.GetPhoneCodeHash(), (*smss)[0].code)
	if err != nil || got != mtproto.BoolTrue {
		t.Fatal(err)
	}
	_, err = svc.ConfirmPhone(ctx, sent.GetPhoneCodeHash(), (*smss)[0].code)
	if rpcText(err) != "CODE_HASH_INVALID" {
		t.Fatal(err)
	}
}

func TestConfirmPhoneExpired(t *testing.T) {
	svc, _, _, _, now := harness(t)
	ctx := context.Background()
	hash, err := svc.IssuePhoneConfirm(ctx, 1, "8613800138000")
	if err != nil {
		t.Fatal(err)
	}
	sent, err := svc.SendConfirmPhoneCode(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Hour)
	_, err = svc.ConfirmPhone(ctx, sent.PhoneCodeHash, "123456")
	if rpcText(err) != "CODE_HASH_INVALID" {
		t.Fatal(err)
	}
}

func TestNewSettingsEmail(t *testing.T) {
	err := validateNewSettings(&mtproto.Account_PasswordInputSettings{
		Email: &wrapperspb.StringValue{Value: "bad"},
	})
	if rpcText(err) != "EMAIL_INVALID" {
		t.Fatal(err)
	}
	algo := mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
		Salt1: []byte{1},
		Salt2: []byte{2},
		G:     3,
		P:     make([]byte, 256),
	}).To_PasswordKdfAlgo()
	if err = validateNewSettings(&mtproto.Account_PasswordInputSettings{
		NewAlgo:         algo,
		NewPasswordHash: make([]byte, 256),
		Email:           &wrapperspb.StringValue{Value: "ok@tellomi.app"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeliverEmailDisabled(t *testing.T) {
	svc := New(nil, Options{SendEmail: func(context.Context, string, string) (string, error) {
		return "", errors.New("resend down")
	}})
	if err := svc.SaveLoginEmail(context.Background(), 1, "a@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RequestPasswordRecovery(context.Background(), 1)
	if err == nil || rpcText(err) == "PASSWORD_RECOVERY_NA" {
		t.Fatal(err)
	}
}

func TestRecoveryUsesCloudPasswordEmail(t *testing.T) {
	ctx := context.Background()
	var code string
	pw := password.New(password.Options{
		SendEmail: func(_ context.Context, to, c string) (string, error) {
			if to != "cloud@tellomi.app" {
				t.Fatalf("confirmation sent to %s", to)
			}
			code = c
			return c, nil
		},
	})
	prev := password.Swap(pw)
	t.Cleanup(func() { password.Swap(prev) })

	issued, err := password.AccountPassword(ctx, 9, 1)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, issued.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{1}, 32)...)
	n, err := password.Update(ctx, 9, 1,
		mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(),
		&mtproto.Account_PasswordInputSettings{
			NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
				Salt1: salt1,
				Salt2: append([]byte{}, issued.GetNewAlgo().GetSalt2()...),
				G:     issued.GetNewAlgo().GetG(),
				P:     append([]byte{}, issued.GetNewAlgo().GetP()...),
			}).To_PasswordKdfAlgo(),
			NewPasswordHash: srp.Verifier([]byte("pw"), salt1, issued.GetNewAlgo().GetSalt2()),
			Email:           &wrapperspb.StringValue{Value: "cloud@tellomi.app"},
		})
	if err != nil || n != 6 {
		t.Fatalf("update n=%d err=%v", n, err)
	}
	if err = password.ConfirmEmail(ctx, 9, code); err != nil {
		t.Fatal(err)
	}

	svc, _, emails, _, _ := harness(t)
	if err = svc.SaveLoginEmail(ctx, 9, "login@tellomi.app"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.RequestPasswordRecovery(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetEmailPattern() != "c***@tellomi.app" || len(*emails) != 1 || (*emails)[0].to != "cloud@tellomi.app" {
		t.Fatalf("%#v %#v", got, *emails)
	}
}
