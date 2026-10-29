package verify

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/verify/aliyun"
)

func TestNewEmailSenderUnsetIsDisabled(t *testing.T) {
	s := NewEmailSender(EmailConfig{})
	if _, ok := s.(DisabledEmail); !ok {
		t.Fatalf("%T", s)
	}
	if err := s.SendCode(context.Background(), "a@b.cn", "12345"); err != ErrEmailDisabled {
		t.Fatal(err)
	}
}

func TestNewEmailSenderLog(t *testing.T) {
	s := NewEmailSender(EmailConfig{Provider: ProviderLog})
	if _, ok := s.(LogEmail); !ok {
		t.Fatalf("%T", s)
	}
	if err := s.SendCode(context.Background(), "a@b.cn", "12345"); err != nil {
		t.Fatal(err)
	}
}

func TestNewEmailSenderAliyunDMDoesNotDial(t *testing.T) {
	s := NewEmailSender(EmailConfig{Provider: ProviderAliyunDM, AccountName: "noreply@mail.tellomi.cn"})
	if _, ok := s.(*aliyun.DirectMailClient); !ok {
		t.Fatalf("%T", s)
	}
}
