package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func testCore() *TwoFaCore {
	return &TwoFaCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
}

func ctor(raw []byte) uint32 {
	return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
}

func flagsOf(raw []byte) uint32 {
	return uint32(raw[4]) | uint32(raw[5])<<8 | uint32(raw[6])<<16 | uint32(raw[7])<<24
}

func TestGetPasswordRoundTrip(t *testing.T) {
	c := testCore()
	pw, err := c.AccountGetPassword(nil)
	if err != nil {
		t.Fatal(err)
	}
	if pw.GetHasPassword() || pw.GetHasRecovery() || pw.GetHasSecureValues() {
		t.Fatalf("flags set on an account with no cloud password: %+v", pw)
	}
	if pw.GetCurrentAlgo() != nil || len(pw.GetSrp_B()) != 0 || pw.GetSrpId() != nil || pw.GetHint() != nil {
		t.Fatal("current password fields must stay unset")
	}
	if pw.GetEmailUnconfirmedPattern() != nil || pw.GetPendingResetDate() != nil || pw.GetLoginEmailPattern() != nil {
		t.Fatal("email and reset fields must stay unset")
	}
	if pw.GetNewAlgo().GetPredicateName() != mtproto.Predicate_passwordKdfAlgoModPow {
		t.Fatalf("new_algo %s", pw.GetNewAlgo().GetPredicateName())
	}
	if pw.GetNewAlgo().GetG() != srpGenerator || len(pw.GetNewAlgo().GetSalt1()) == 0 || len(pw.GetNewAlgo().GetSalt2()) == 0 {
		t.Fatal("new_algo parameters")
	}
	if pw.GetNewSecureAlgo().GetPredicateName() != mtproto.Predicate_securePasswordKdfAlgoPBKDF2 {
		t.Fatalf("new_secure_algo %s", pw.GetNewSecureAlgo().GetPredicateName())
	}
	if len(pw.GetSecureRandom()) != secureRandomBytes {
		t.Fatalf("secure_random %d", len(pw.GetSecureRandom()))
	}

	buf := mtproto.NewEncodeBuf(1024)
	if err := pw.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if ctor(raw) != 0x957b50fb {
		t.Fatalf("constructor %x", ctor(raw))
	}
	if flagsOf(raw) != 0 {
		t.Fatalf("flags %x", flagsOf(raw))
	}
	decoded := &mtproto.Account_Password{}
	if err := decoded.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if decoded.GetNewAlgo() == nil || decoded.GetNewSecureAlgo() == nil || len(decoded.GetSecureRandom()) != secureRandomBytes {
		t.Fatal("decode dropped required fields")
	}
}

func TestPasswordSettingsAndEmailFlows(t *testing.T) {
	c := testCore()
	empty := mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP()
	settings, err := c.AccountGetPasswordSettings(&mtproto.TLAccountGetPasswordSettings{Password: empty})
	if err != nil {
		t.Fatal(err)
	}
	buf := mtproto.NewEncodeBuf(64)
	if err := settings.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if ctor(raw) != 0x9a5c33e5 || flagsOf(raw) != 0 {
		t.Fatalf("passwordSettings ctor %x flags %x", ctor(raw), flagsOf(raw))
	}

	srp := mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: 1, A: []byte{1}, M1: []byte{1},
	}).To_InputCheckPasswordSRP()
	if _, err := c.AccountGetPasswordSettings(&mtproto.TLAccountGetPasswordSettings{Password: srp}); err != mtproto.ErrPasswordHashInvalid {
		t.Fatalf("srp settings: %v", err)
	}
	if _, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{Password: srp}); err != mtproto.ErrSrpIdInvalid {
		t.Fatalf("srp update: %v", err)
	}
	if _, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{Password: empty}); err != mtproto.ErrNewSettingsEmpty {
		t.Fatalf("empty update: %v", err)
	}

	badEmail := &mtproto.Account_PasswordInputSettings{
		NewAlgo:         c.accountPassword().GetNewAlgo(),
		NewPasswordHash: make([]byte, passwordHashLen),
		Email:           &wrapperspb.StringValue{Value: "not-an-email"},
	}
	if _, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password: empty, NewSettings: badEmail,
	}); err != mtproto.ErrEmailInvalid {
		t.Fatalf("email: %v", err)
	}
	enroll := &mtproto.Account_PasswordInputSettings{
		NewAlgo:         c.accountPassword().GetNewAlgo(),
		NewPasswordHash: make([]byte, passwordHashLen),
		Hint:            &wrapperspb.StringValue{Value: "hint"},
	}
	if _, err := c.AccountUpdatePasswordSettings(&mtproto.TLAccountUpdatePasswordSettings{
		Password: empty, NewSettings: enroll,
	}); err != mtproto.ErrNewSettingsInvalid {
		t.Fatalf("enroll: %v", err)
	}

	if _, err := c.AccountConfirmPasswordEmail(&mtproto.TLAccountConfirmPasswordEmail{Code: "12345"}); err != mtproto.ErrEmailHashExpired {
		t.Fatalf("confirm: %v", err)
	}
	if _, err := c.AccountResendPasswordEmail(nil); err != mtproto.ErrEmailHashExpired {
		t.Fatalf("resend: %v", err)
	}
	if _, err := c.AccountCancelPasswordEmail(nil); err != mtproto.ErrEmailHashExpired {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := c.AccountDeclinePasswordReset(nil); err != mtproto.ErrResetRequestMissing {
		t.Fatalf("decline: %v", err)
	}
}

func TestGetTmpPassword(t *testing.T) {
	c := testCore()
	empty := mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP()
	got, err := c.AccountGetTmpPassword(&mtproto.TLAccountGetTmpPassword{Password: empty, Period: 1800})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetTmpPassword()) != tmpPasswordBytes {
		t.Fatalf("tmp len %d", len(got.GetTmpPassword()))
	}
	if got.GetValidUntil() < int32(time.Now().Unix())+1790 {
		t.Fatalf("valid_until %d", got.GetValidUntil())
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if ctor(buf.GetBuf()) != 0xdb64fd34 {
		t.Fatalf("constructor %x", ctor(buf.GetBuf()))
	}

	short, err := c.AccountGetTmpPassword(&mtproto.TLAccountGetTmpPassword{Password: empty, Period: 1})
	if err != nil {
		t.Fatal(err)
	}
	if short.GetValidUntil() > int32(time.Now().Unix())+tmpPasswordPeriodMin+5 {
		t.Fatal("period was not clamped to the documented minimum")
	}
	if _, err := c.AccountGetTmpPassword(&mtproto.TLAccountGetTmpPassword{
		Password: mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{SrpId: 4}).To_InputCheckPasswordSRP(),
		Period:   60,
	}); err != errSRPAInvalid {
		t.Fatalf("empty A: %v", err)
	}
}
