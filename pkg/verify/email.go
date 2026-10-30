package verify

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	ErrEmailDisabled = errors.New("email verification is disabled")
	ErrInvalidEmail  = errors.New("invalid email")
)

// CodeSender delivers a numeric verification code.
type CodeSender interface {
	SendCode(ctx context.Context, to, code string) error
}

// LogEmail prints the code and does not call a provider. It is the default
// when TELLOMI_EMAIL_PROVIDER is unset, and when it is explicitly "log".
type LogEmail struct{}

func (LogEmail) SendCode(ctx context.Context, to, code string) error {
	logx.WithContext(ctx).Infof("tellomi email verification code to=%s code=%s", NormalizeEmail(to), code)
	return nil
}

// DisabledEmail refuses to send. Used when TELLOMI_EMAIL_PROVIDER is
// disabled, none, off, or an unknown value such as aliyun_dm.
type DisabledEmail struct{}

func (DisabledEmail) SendCode(context.Context, string, string) error {
	return ErrEmailDisabled
}

// NormalizeEmail trims and lower-cases an address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail is a practical check for a single mailbox, not a full RFC parser.
func ValidEmail(email string) bool {
	email = NormalizeEmail(email)
	if email == "" || strings.ContainsAny(email, " \t\r\n<>") {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return false
	}
	local, domain := email[:at], email[at+1:]
	if local == "" || domain == "" || strings.Contains(domain, "@") {
		return false
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return utf8.ValidString(email)
}

// IsEmail reports whether raw looks like an email address rather than a phone.
func IsEmail(raw string) bool {
	return strings.Contains(raw, "@")
}

// MaskEmail hides the local part the way auth.sentCodeTypeEmailCode clients expect.
func MaskEmail(email string) string {
	email = NormalizeEmail(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	r, _ := utf8.DecodeRuneInString(local)
	if r == utf8.RuneError {
		return "***" + domain
	}
	return string(r) + "***" + domain
}
