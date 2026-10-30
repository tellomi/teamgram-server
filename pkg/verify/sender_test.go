package verify

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/verify/postal"
	"github.com/teamgram/teamgram-server/pkg/verify/resend"
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

func TestNewEmailSenderAliyunDMIsDisabled(t *testing.T) {
	s := NewEmailSender(EmailConfig{Provider: ProviderAliyunDM, AccountName: "noreply@mail.tellomi.cn"})
	if _, ok := s.(DisabledEmail); !ok {
		t.Fatalf("%T", s)
	}
}

func TestNewEmailSenderResendDoesNotDial(t *testing.T) {
	s := NewEmailSender(EmailConfig{
		Provider: ProviderResend,
		From:     DefaultEmailFrom,
		FromName: DefaultEmailFromName,
	})
	c, ok := s.(*resend.Client)
	if !ok {
		t.Fatalf("%T", s)
	}
	if c.APIKey != "" {
		t.Fatal("api key must come from config")
	}
	if err := c.SendCode(context.Background(), "a@b.cn", "123456"); err == nil {
		t.Fatal("missing api key must fail before a successful send")
	}
}

func TestNewEmailSenderPostalDoesNotDial(t *testing.T) {
	s := NewEmailSender(EmailConfig{
		Provider:      ProviderPostal,
		From:          DefaultEmailFrom,
		FromName:      DefaultEmailFromName,
		PostalBaseURL: "https://postal.internal",
	})
	c, ok := s.(*postal.Client)
	if !ok {
		t.Fatalf("%T", s)
	}
	if err := c.SendCode(context.Background(), "a@b.cn", "123456"); err == nil {
		t.Fatal("missing api key must fail before a successful send")
	}
}
