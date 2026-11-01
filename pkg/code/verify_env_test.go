package code

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

func TestFixedProviderSendDoesNotWait(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "")
	v := NewVerifyCode(&conf.SmsVerifyCodeConfig{Name: "none", FixedCode: "12345"})
	start := time.Now()
	extra, err := v.SendSmsVerifyCode(context.Background(), "8613800138000", "12345", "h")
	if err != nil || extra != "12345" {
		t.Fatalf("fixed send: extra=%q err=%v", extra, err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("fixed provider waited %s", time.Since(start))
	}
}

func TestAliyunMissingKeysFailFast(t *testing.T) {
	t.Setenv("TELLOMI_SMS_PROVIDER", "aliyun")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
	t.Setenv("TELLOMI_SMS_ALLOW_INTERNATIONAL", "false")
	v := NewVerifyCode(&conf.SmsVerifyCodeConfig{Name: "none"})
	start := time.Now()
	_, err := v.SendSmsVerifyCode(context.Background(), "8613800138000", "123456", "h")
	if err == nil || !strings.Contains(err.Error(), "access key") {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("missing keys waited %s", time.Since(start))
	}
}

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
