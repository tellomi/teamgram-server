package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/app/bff/codelogin"
	"google.golang.org/grpc/status"
)

// The P0 login-code RPCs share one in-memory service so a case can walk
// send, verify, and the documented error the way a client does. Delivery
// is a fake of the existing email and SMS senders.

type delivery struct {
	emails []string
	sms    []string
}

func newLogin(t *testing.T, d *delivery) (*codelogin.Service, *codelogin.MemPhones) {
	t.Helper()
	phones := codelogin.NewMemPhones()
	svc := codelogin.New(nil, codelogin.Options{
		EmailLen:    6,
		SMSLen:      6,
		TTL:         3 * time.Minute,
		MaxAttempts: 5,
		Phones:      phones,
		Now:         time.Now,
		LookupUser:  func(context.Context, string) (int64, error) { return 42, nil },
		SendEmail: func(_ context.Context, to, code string) (string, error) {
			d.emails = append(d.emails, to+"="+code)
			return code, nil
		},
		SendSMS: func(_ context.Context, phone, code string) (string, error) {
			d.sms = append(d.sms, phone+"="+code)
			return code, nil
		},
	})
	return svc, phones
}

func rpc(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func TestP0EmailSetupRecoveryAndReset(t *testing.T) {
	var d delivery
	svc, phones := newLogin(t, &d)
	ctx := context.Background()
	phones.Put(&codelogin.PhoneCode{
		AuthKeyID: 9,
		Phone:     "8613800138000",
		Hash:      "phone-hash",
		Code:      "111111",
		Extra:     "111111",
		Expires:   time.Now().Add(time.Hour).Unix(),
		SentType:  model.SentCodeTypeSms,
		UserID:    42,
	})
	purpose := mtproto.MakeTLEmailVerifyPurposeLoginSetup(&mtproto.EmailVerifyPurpose{
		PhoneNumber:   "8613800138000",
		PhoneCodeHash: "phone-hash",
	}).To_EmailVerifyPurpose()
	sent, err := svc.SendVerifyEmailCode(ctx, 9, 0, &mtproto.TLAccountSendVerifyEmailCode{
		Purpose: purpose,
		Email:   "owner@tellomi.app",
	})
	if err != nil || sent.EmailPattern != "o***@tellomi.app" || len(d.emails) != 1 {
		t.Fatalf("send verify email: %v %#v %#v", err, sent, d.emails)
	}
	code := d.emails[0][len("owner@tellomi.app="):]
	verified, err := svc.VerifyEmail(ctx, 9, 0, &mtproto.TLAccountVerifyEmail32DA4CF{
		Purpose: purpose,
		Verification: mtproto.MakeTLEmailVerificationCode(&mtproto.EmailVerification{
			Code: code,
		}).To_EmailVerification(),
	})
	if err != nil || verified.PredicateName != mtproto.Predicate_account_emailVerifiedLogin {
		t.Fatalf("verify: %v %#v", err, verified)
	}
	if verified.SentCode.Type.PredicateName != mtproto.Predicate_auth_sentCodeTypeSms || len(d.sms) != 1 {
		t.Fatalf("next sms %#v %#v", verified.SentCode, d.sms)
	}

	recovery, err := svc.RequestPasswordRecovery(ctx, 42)
	if err != nil || recovery.EmailPattern != "o***@tellomi.app" {
		t.Fatalf("recovery: %v %#v", err, recovery)
	}
	recoverCode := d.emails[len(d.emails)-1][len("owner@tellomi.app="):]
	if _, err = svc.CheckRecoveryPassword(ctx, 42, "000000"); rpc(err) != "CODE_INVALID" {
		t.Fatal(err)
	}
	if _, err = svc.CheckRecoveryPassword(ctx, 42, recoverCode); err != nil {
		t.Fatal(err)
	}
	if err = svc.RecoverPassword(ctx, 42, recoverCode, nil); err != nil {
		t.Fatal(err)
	}

	reset, err := svc.ResetLoginEmail(ctx, 9, "8613800138000", "phone-hash", 42)
	if err != nil || reset.Type.PredicateName != mtproto.Predicate_auth_sentCodeTypeEmailCode {
		t.Fatalf("reset: %v %#v", err, reset)
	}
	if _, err = svc.ResetLoginEmail(ctx, 9, "8613800138000", "phone-hash", 42); rpc(err) != "TASK_ALREADY_EXISTS" {
		t.Fatal(err)
	}
}

func TestP0FirebaseReportInvalidateConfirm(t *testing.T) {
	var d delivery
	svc, phones := newLogin(t, &d)
	ctx := context.Background()
	phones.Put(&codelogin.PhoneCode{
		AuthKeyID: 3,
		Phone:     "8613800138000",
		Hash:      "h",
		Code:      "654321",
		Extra:     "654321",
		Expires:   time.Now().Add(time.Hour).Unix(),
		SentType:  model.SentCodeTypeSms,
		UserID:    7,
	})
	fallback, err := svc.RequestFirebaseSms(ctx, 3, "8613800138000", "h")
	if err != nil || fallback != mtproto.BoolFalse {
		t.Fatalf("firebase fallback: %v %v", fallback, err)
	}
	_, err = svc.RequestFirebaseSms(ctx, 3, "nope", "h")
	if rpc(err) != "PHONE_NUMBER_INVALID" {
		t.Fatal(err)
	}
	logged := ""
	ok, err := svc.ReportMissingCode("8613800138000", "h", "00", func(phone, hash, mnc string) {
		logged = phone + "/" + hash + "/" + mnc
	})
	if err != nil || ok != mtproto.BoolTrue || logged != "8613800138000/h/00" {
		t.Fatalf("report: %v %v %s", ok, err, logged)
	}
	if err = svc.RememberSignInCode(ctx, 7, 3, "8613800138000", "654321", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.InvalidateSignInCodes(ctx, 7, []string{"654321"}); err != nil {
		t.Fatal(err)
	}
	if pc, _ := phones.Get(ctx, 3, "8613800138000", "h"); pc != nil {
		t.Fatal("invalidated code still stored")
	}

	link, err := svc.IssuePhoneConfirm(ctx, 7, "8613800138000")
	if err != nil {
		t.Fatal(err)
	}
	sent, err := svc.SendConfirmPhoneCode(ctx, link)
	if err != nil || len(d.sms) != 1 {
		t.Fatalf("confirm send: %v %#v %#v", err, sent, d.sms)
	}
	smsCode := d.sms[0][len("8613800138000="):]
	if _, err = svc.ConfirmPhone(ctx, sent.PhoneCodeHash, "000000"); rpc(err) != "PHONE_CODE_INVALID" {
		t.Fatal(err)
	}
	if _, err = svc.ConfirmPhone(ctx, sent.PhoneCodeHash, smsCode); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SendConfirmPhoneCode(ctx, "unknown"); rpc(err) != "HASH_INVALID" {
		t.Fatal(err)
	}
}
