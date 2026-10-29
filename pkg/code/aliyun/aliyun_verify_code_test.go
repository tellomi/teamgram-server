package aliyun

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/verify"
)

type mockSender struct {
	phone string
	code  string
	err   error
}

func (m *mockSender) SendCode(ctx context.Context, phone, code string) error {
	m.phone = phone
	m.code = code
	return m.err
}

func TestMockSenderAndVerify(t *testing.T) {
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "false")
	sender := &mockSender{}
	v := NewWithSender(sender)
	extra, err := v.SendSmsVerifyCode(context.Background(), "8613800138000", "123456", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if sender.phone != "13800138000" || sender.code != "123456" || extra != "123456" {
		t.Fatalf("sent %+v extra %s", sender, extra)
	}
	if err := v.VerifySmsCode(context.Background(), "hash", "123456", extra); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifySmsCode(context.Background(), "hash", "000000", extra); err == nil {
		t.Fatal("expected invalid code")
	}
}

func TestInternationalRejectedUnlessEnabled(t *testing.T) {
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "false")
	v := NewWithSender(&mockSender{})
	_, err := v.SendSmsVerifyCode(context.Background(), "14155552671", "12345", "h")
	if err != verify.ErrInternationalDisabled {
		t.Fatal(err)
	}
}

func TestSenderError(t *testing.T) {
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "false")
	v := NewWithSender(&mockSender{err: errors.New("down")})
	if _, err := v.SendSmsVerifyCode(context.Background(), "8613800138000", "12345", "h"); err == nil {
		t.Fatal("expected send error")
	}
}
