package none

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/code/conf"
)

func TestFixedCodeDefaultsTo12345(t *testing.T) {
	v := New(&conf.SmsVerifyCodeConfig{Name: "none"})
	if err := v.VerifySmsCode(context.Background(), "hash", "12345", ""); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifySmsCode(context.Background(), "hash", "00000", ""); err == nil {
		t.Fatal("expected invalid code")
	}
}

func TestFixedCodeIsConfigurable(t *testing.T) {
	v := New(&conf.SmsVerifyCodeConfig{Name: "none", FixedCode: "4242"})
	if err := v.VerifySmsCode(context.Background(), "hash", "4242", ""); err != nil {
		t.Fatal(err)
	}
	if err := v.VerifySmsCode(context.Background(), "hash", "12345", ""); err == nil {
		t.Fatal("default code must not be accepted once FixedCode is set")
	}
}
