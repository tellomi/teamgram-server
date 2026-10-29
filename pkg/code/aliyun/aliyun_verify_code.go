package aliyun

import (
	"context"
	"crypto/subtle"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/teamgram/teamgram-server/pkg/verify"
	aliyunapi "github.com/teamgram/teamgram-server/pkg/verify/aliyun"
)

// Sender delivers an SMS verification code. Tests pass a fake.
type Sender interface {
	SendCode(ctx context.Context, phone, code string) error
}

type verifyCode struct {
	sender Sender
}

// New builds the Aliyun Dysmsapi sender from process configuration.
func New(c *conf.SmsVerifyCodeConfig) *verifyCode {
	_ = c
	cfg := verify.Load()
	return NewWithSender(&aliyunapi.SMSClient{
		AccessKeyID:     cfg.SMS.AccessKeyID,
		AccessKeySecret: cfg.SMS.AccessKeySecret,
		Region:          cfg.SMS.Region,
		SignName:        cfg.SMS.SignName,
		TemplateCode:    cfg.SMS.TemplateCode,
		Endpoint:        cfg.SMS.Endpoint,
	})
}

// NewWithSender wires an arbitrary SMS sender. The code checked at sign-in is
// the value returned from SendSmsVerifyCode (the code that was sent).
func NewWithSender(sender Sender) *verifyCode {
	return &verifyCode{sender: sender}
}

func (m *verifyCode) SendSmsVerifyCode(ctx context.Context, phoneNumber, code, codeHash string) (string, error) {
	_ = codeHash
	if m == nil || m.sender == nil {
		return "", mtproto.ErrSmsCodeCreateFailed
	}
	phone, err := verify.FormatAliyunPhone(phoneNumber, verify.Load().SMS.AllowInternational)
	if err != nil {
		return "", err
	}
	if err = m.sender.SendCode(ctx, phone, code); err != nil {
		return "", err
	}
	return code, nil
}

func (m *verifyCode) VerifySmsCode(ctx context.Context, codeHash, code, extraData string) error {
	_ = ctx
	_ = codeHash
	if code == "" || extraData == "" || subtle.ConstantTimeCompare([]byte(code), []byte(extraData)) != 1 {
		return mtproto.ErrPhoneCodeInvalid
	}
	return nil
}
