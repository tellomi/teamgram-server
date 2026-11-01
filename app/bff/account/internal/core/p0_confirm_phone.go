package core

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/codelogin"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/teamgram/teamgram-server/pkg/verify"
)

func (c *AccountCore) confirmCodes() *codelogin.Service {
	var kv codelogin.KV
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil {
		kv = c.svcCtx.Dao.Cache()
	}
	cfg := verify.Load()
	smsLen := cfg.SMS.CodeLength
	if smsLen <= 0 {
		smsLen = 5
	}
	ttl := time.Duration(cfg.Limits.TTLSeconds) * time.Second
	return codelogin.New(kv, codelogin.Options{
		SMSLen:      smsLen,
		TTL:         ttl,
		MaxAttempts: cfg.Limits.MaxAttempts,
		NormalizePhone: func(phone string) (string, error) {
			_, n, err := phonenumber.CheckPhoneNumberInvalid(phone)
			if err != nil {
				return "", mtproto.ErrPhoneNumberInvalid
			}
			return n, nil
		},
		SendSMS: c.deliverConfirmSMS,
	})
}

// deliverConfirmSMS uses the same Aliyun login template as auth.sendCode
// and account.sendChangePhoneCode. The call is capped at SMSSendTimeout.
func (c *AccountCore) deliverConfirmSMS(ctx context.Context, phone, code string) (string, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.AuthLogic == nil || c.svcCtx.AuthLogic.VerifyCodeInterface == nil {
		return "", mtproto.ErrSmsCodeCreateFailed
	}
	cfg := verify.Load()
	if cfg.SMS.Provider == verify.ProviderAliyun {
		if _, err := verify.FormatAliyunPhone(phone, cfg.SMS.AllowInternational); err != nil {
			return "", mtproto.ErrPhoneNumberInvalid
		}
		if c.svcCtx.Dao != nil && c.svcCtx.Dao.Cache() != nil {
			if err := verify.NewLimiter(c.svcCtx.Dao.Cache(), cfg.Limits).AllowSend(ctx, "sms", phone, confirmClientIP(c)); err != nil {
				var lim *verify.LimitError
				if errors.As(err, &lim) {
					if lim.Daily {
						return "", mtproto.ErrPhoneNumberFlood
					}
					retry := lim.Retry
					if retry <= 0 {
						retry = cfg.Limits.ResendSeconds
					}
					if retry <= 0 {
						retry = 60
					}
					return "", mtproto.NewErrFloodWaitX(int32(retry))
				}
				return "", err
			}
		}
	}
	timeout := verify.SMSSendTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	extra, err := c.svcCtx.AuthLogic.VerifyCodeInterface.SendSmsVerifyCode(sendCtx, phone, code, "confirm-phone")
	if err != nil {
		if errors.Is(err, verify.ErrInternationalDisabled) {
			return "", mtproto.ErrPhoneNumberInvalid
		}
		c.Logger.Errorf("account.sendConfirmPhoneCode send sms: %v", err)
		return "", mtproto.ErrSmsCodeCreateFailed
	}
	if extra == "" {
		extra = code
	}
	return extra, nil
}

func confirmClientIP(c *AccountCore) string {
	if c == nil || c.MD == nil {
		return ""
	}
	return c.MD.ClientAddr
}
