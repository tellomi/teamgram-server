package core

import (
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

// guardSMS applies the +86 rule and send quotas. Fixed mode and test numbers
// never reach it. Test numbers return before the SMS branch.
func (c *AuthorizationCore) guardSMS(phone string) error {
	if !c.aliyunSMS() {
		return nil
	}
	if _, err := verify.FormatAliyunPhone(phone, c.svcCtx.Verify.SMS.AllowInternational); err != nil {
		return mtproto.ErrPhoneNumberInvalid
	}
	if c.svcCtx.Limiter == nil {
		return nil
	}
	err := c.svcCtx.Limiter.AllowSend(c.ctx, "sms", phone, c.clientIP())
	if err == nil {
		return nil
	}
	var lim *verify.LimitError
	if errors.As(err, &lim) {
		if lim.Daily {
			return mtproto.ErrPhoneNumberFlood
		}
		retry := lim.Retry
		if retry <= 0 {
			retry = c.svcCtx.Verify.Limits.ResendSeconds
		}
		if retry <= 0 {
			retry = 60
		}
		return mtproto.NewErrFloodWaitX(int32(retry))
	}
	return err
}

func (c *AuthorizationCore) sendSMS(phone string, codeData *model.PhoneCodeTransaction) error {
	if err := c.guardSMS(phone); err != nil {
		return err
	}
	c.prepareSMSCode(codeData)
	extra, err := c.svcCtx.AuthLogic.VerifyCodeInterface.SendSmsVerifyCode(
		c.ctx, phone, codeData.PhoneCode, codeData.PhoneCodeHash)
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
