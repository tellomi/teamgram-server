package core

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/pkg/verify"
)

func (c *AuthorizationCore) clientIP() string {
	if c.MD == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(c.MD.ClientAddr)
	if err != nil {
		return c.MD.ClientAddr
	}
	return host
}

func (c *AuthorizationCore) aliyunSMS() bool {
	return c.svcCtx != nil && c.svcCtx.Verify.SMS.Provider == verify.ProviderAliyun
}

// prepareSMSCode sizes the code to TELLOMI_SMS_CODE_LENGTH when Aliyun SMS is on.
// Fixed mode keeps the code CreatePhoneCode already stored.
func (c *AuthorizationCore) prepareSMSCode(codeData *model.PhoneCodeTransaction) {
	if !c.aliyunSMS() || codeData == nil {
		return
	}
	n := c.svcCtx.Verify.SMS.CodeLength
	if len(codeData.PhoneCode) != n {
		codeData.PhoneCode = verify.RandomDigits(n)
	}
	ttl := c.svcCtx.Verify.Limits.TTLSeconds
	if ttl <= 0 {
		ttl = 180
	}
	codeData.PhoneCodeExpired = int32(time.Now().Unix()) + int32(ttl)
}

// guardSMS applies the country allow list and the daily quota before any
// provider call. Test numbers return before this. Fixed mode still counts,
// and still refuses a number outside the allow list, so a dev sender is not
// a way around the cap.
func (c *AuthorizationCore) guardSMS(phone string) error {
	if c.svcCtx == nil {
		return nil
	}
	sms := c.svcCtx.Verify.SMS
	if err := verify.RequireCountry(phone, sms.AllowInternational, sms.Countries); err != nil {
		return mtproto.ErrPhoneNumberInvalid
	}
	lim := c.svcCtx.Limiter
	if lim == nil {
		lim = verify.SharedSMS()
	}
	if lim == nil {
		return nil
	}
	err := lim.AllowSend(c.ctx, "sms", phone, c.clientIP())
	if err == nil {
		return nil
	}
	return verify.PhoneQuotaError(err)
}

func (c *AuthorizationCore) sendSMS(phone string, codeData *model.PhoneCodeTransaction) error {
	if err := c.guardSMS(phone); err != nil {
		return err
	}
	c.prepareSMSCode(codeData)
	// Bound the provider call. Aliyun errors, missing keys, and a stalled
	// TCP connection all become SMS_CODE_CREATE_FAILED inside this window
	// so auth.sendCode does not leave the client spinning.
	ctx, cancel := context.WithTimeout(c.requestCtx(), smsSendTimeout())
	defer cancel()
	extra, err := c.svcCtx.AuthLogic.VerifyCodeInterface.SendSmsVerifyCode(
		ctx, phone, codeData.PhoneCode, codeData.PhoneCodeHash)
	if err != nil {
		if errors.Is(err, verify.ErrInternationalDisabled) {
			return mtproto.ErrPhoneNumberInvalid
		}
		c.Logger.Errorf("send sms code: %v", err)
		return mtproto.ErrSmsCodeCreateFailed
	}
	codeData.SentCodeType = model.SentCodeTypeSms
	codeData.PhoneCodeExtraData = extra
	return nil
}

func (c *AuthorizationCore) requestCtx() context.Context {
	if c != nil && c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func smsSendTimeout() time.Duration {
	if verify.SMSSendTimeout <= 0 {
		return 10 * time.Second
	}
	return verify.SMSSendTimeout
}
