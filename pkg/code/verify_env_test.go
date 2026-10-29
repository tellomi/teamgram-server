package code

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

func TestProviderFromEnv(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "")
	fixed := NewVerifyCode(&conf.SmsVerifyCodeConfig{Name: "none"})
	if err := fixed.VerifySmsCode(context.Background(), "h", "12345", "99999"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TELLOMI_SMS_PROVIDER", "aliyun")
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "false")
	real := NewVerifyCode(&conf.SmsVerifyCodeConfig{Name: "none"})
	if err := real.VerifySmsCode(context.Background(), "h", "12345", "12345"); err != nil {
		t.Fatal(err)
	}
	if err := real.VerifySmsCode(context.Background(), "h", "12345", "99999"); err == nil {
		t.Fatal("aliyun provider must check the sent code")
	}
	if err := fixed.VerifySmsCode(context.Background(), "h", "12345", "99999"); err != nil {
		t.Fatal("existing fixed verifier must keep accepting 12345")
	}
}
