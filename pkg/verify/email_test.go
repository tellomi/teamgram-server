package verify

import (
	"context"
	"testing"
)

func TestEmailValidationAndMask(t *testing.T) {
	if !ValidEmail(" User@Mail.Tellomi.CN ") {
		t.Fatal("expected valid")
	}
	if ValidEmail("not-an-email") || ValidEmail("a@b") || ValidEmail("@x.cn") {
		t.Fatal("expected invalid")
	}
	if got := NormalizeEmail(" User@Mail.Tellomi.CN "); got != "user@mail.tellomi.cn" {
		t.Fatal(got)
	}
	if got := MaskEmail("user@mail.tellomi.cn"); got != "u***@mail.tellomi.cn" {
		t.Fatal(got)
	}
	if !IsEmail("a@b.cn") || IsEmail("+8613800138000") {
		t.Fatal("is email")
	}
}

func TestDisabledEmailDoesNotSend(t *testing.T) {
	err := DisabledEmail{}.SendCode(context.Background(), "a@b.cn", "12345")
	if err != ErrEmailDisabled {
		t.Fatal(err)
	}
}

func TestLogEmailAcceptsCode(t *testing.T) {
	if err := (LogEmail{}).SendCode(context.Background(), "a@b.cn", "12345"); err != nil {
		t.Fatal(err)
	}
}
