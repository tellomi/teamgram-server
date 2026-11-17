package core

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/teamgram/marmota/pkg/threading2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/logic"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func normalizeEmailLogin(raw string) (string, error) {
	email := verify.NormalizeEmail(raw)
	if !verify.ValidEmail(email) || len(email) > 255 {
		return "", mtproto.ErrEmailInvalid
	}
	return email, nil
}

// emailLoginCode accepts emailVerificationCode, or phone_code when the
// client did not send email_verification. Apple and Google tokens are
// EMAIL_INVALID. An empty emailVerificationCode is CODE_INVALID.
func emailLoginCode(in *mtproto.TLAuthSignIn) (string, error) {
	if in == nil {
		return "", mtproto.ErrPhoneCodeEmpty
	}
	if ev := in.GetEmailVerification(); ev != nil {
		name := ev.GetPredicateName()
		ctor := ev.GetConstructor()
		switch {
		case name == mtproto.Predicate_emailVerificationCode || ctor == mtproto.CRC32_emailVerificationCode:
			if ev.GetCode() == "" {
				return "", mtproto.ErrCodeInvalid
			}
			return ev.GetCode(), nil
		case name == mtproto.Predicate_emailVerificationGoogle || ctor == mtproto.CRC32_emailVerificationGoogle:
			return "", mtproto.ErrEmailInvalid
		case name == mtproto.Predicate_emailVerificationApple || ctor == mtproto.CRC32_emailVerificationApple:
			return "", mtproto.ErrEmailInvalid
		default:
			if name != "" || ctor != 0 {
				return "", mtproto.ErrEmailInvalid
			}
		}
	}
	code := in.GetPhoneCode_STRING()
	if code == "" && in.GetPhoneCode_FLAGSTRING() != nil {
		code = in.GetPhoneCode_FLAGSTRING().GetValue()
	}
	if code == "" {
		return "", mtproto.ErrPhoneCodeEmpty
	}
	return code, nil
}

// signInCode reads the login code for a phone or for an email-as-phone
// account. A real phone number with flags.1 set uses email_verification.
func signInCode(in *mtproto.TLAuthSignIn) (string, error) {
	if in == nil {
		return "", mtproto.ErrPhoneCodeEmpty
	}
	if verify.IsEmail(in.GetPhoneNumber()) || in.GetEmailVerification() != nil {
		return emailLoginCode(in)
	}
	code := in.GetPhoneCode_STRING()
	if code == "" && in.GetPhoneCode_FLAGSTRING() != nil {
		code = in.GetPhoneCode_FLAGSTRING().GetValue()
	}
	if code == "" {
		return "", mtproto.ErrPhoneCodeEmpty
	}
	return code, nil
}

func storedCodeMatches(sent, got string) bool {
	if sent == "" || len(sent) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sent), []byte(got)) == 1
}

func (c *AuthorizationCore) prepareEmailCode(codeData *model.PhoneCodeTransaction) {
	if codeData == nil || c.svcCtx == nil {
		return
	}
	n := c.svcCtx.Verify.Email.CodeLength
	if n <= 0 {
		n = 6
	}
	if len(codeData.PhoneCode) != n {
		codeData.PhoneCode = verify.RandomDigits(n)
	}
	ttl := c.svcCtx.Verify.EmailLimits.TTLSeconds
	if ttl <= 0 {
		ttl = 180
	}
	codeData.PhoneCodeExpired = int32(time.Now().Unix()) + int32(ttl)
}

func (c *AuthorizationCore) guardEmail(email string) error {
	if c.svcCtx == nil {
		return nil
	}
	lim := c.svcCtx.EmailLimiter
	if lim == nil {
		lim = verify.SharedEmail()
	}
	if lim == nil {
		return nil
	}
	err := lim.AllowSend(c.ctx, "email", email, c.clientIP())
	if err == nil {
		return nil
	}
	return verify.EmailQuotaError(err)
}

func (c *AuthorizationCore) sendEmail(email string, codeData *model.PhoneCodeTransaction) error {
	if c.svcCtx == nil || !c.svcCtx.Verify.Email.Enabled() {
		return mtproto.ErrEmailNotSetup
	}
	if err := c.guardEmail(email); err != nil {
		return err
	}
	c.prepareEmailCode(codeData)
	sender := c.svcCtx.EmailSender
	if sender == nil {
		sender = verify.NewEmailSender(c.svcCtx.Verify.Email)
	}
	if err := sender.SendCode(c.ctx, email, codeData.PhoneCode); err != nil {
		if errors.Is(err, verify.ErrEmailDisabled) {
			return mtproto.ErrEmailNotSetup
		}
		c.Logger.Errorf("send email code: %v", err)
		return mtproto.ErrSmsCodeCreateFailed
	}
	codeData.SentCodeType = model.SentCodeTypeEmailCode
	codeData.NextCodeType = model.CodeTypeNone
	codeData.EmailPattern = verify.MaskEmail(email)
	codeData.PhoneCodeExtraData = codeData.PhoneCode
	codeData.State = model.CodeStateSent
	return nil
}

func (c *AuthorizationCore) finishEmailSentCode(codeData *model.PhoneCodeTransaction) *mtproto.Auth_SentCode {
	sent := codeData.ToAuthSentCode()
	if sent == nil {
		return nil
	}
	sec := 60
	if c.svcCtx != nil && c.svcCtx.Verify.EmailLimits.ResendSeconds > 0 {
		sec = c.svcCtx.Verify.EmailLimits.ResendSeconds
	}
	sent.Timeout = &wrapperspb.Int32Value{Value: int32(sec)}
	return sent
}

// authSendEmailCode is auth.sendCode when phone_number is an email address.
// The code is stored on the same phone-code record as SMS and delivered by
// the configured email sender. Unregistered addresses continue to auth.signUp
// with no password.
func (c *AuthorizationCore) authSendEmailCode(authKeyId, sessionId int64, request *mtproto.TLAuthSendCode) (reply *mtproto.Auth_SentCode, err error) {
	email, err := normalizeEmailLogin(request.PhoneNumber)
	if err != nil {
		c.Logger.Errorf("auth.sendCode - email invalid")
		return nil, err
	}

	if c.svcCtx.Plugin != nil {
		banned, _ := c.svcCtx.Plugin.CheckPhoneNumberBanned(c.ctx, email)
		if banned {
			c.Logger.Errorf("auth.sendCode - email banned")
			return nil, mtproto.ErrPhoneNumberBanned
		}
	}

	actionType := logic.GetActionType(request)
	if err = c.svcCtx.Dao.CheckCanDoAction(c.ctx, authKeyId, email, actionType); err != nil {
		c.Logger.Errorf("auth.sendCode - email action: %v", err)
		return nil, err
	}

	var (
		registered = false
		user       *mtproto.ImmutableUser
	)
	if user, err = c.svcCtx.Dao.UserClient.UserGetImmutableUserByPhone(c.ctx, &userpb.TLUserGetImmutableUserByPhone{
		Phone: email,
	}); err != nil {
		if _, ok := status.FromError(err); ok {
			err = nil
		} else {
			c.Logger.Errorf("auth.sendCode - email lookup: %v", err)
			return nil, err
		}
	} else {
		registered = true
	}

	if registered && user != nil {
		for _, v := range request.Settings.GetLogoutTokens() {
			id, _ := c.svcCtx.Dao.GetFutureAuthToken(c.ctx, v)
			if id != user.Id() {
				continue
			}
			if c.needsCloudPassword(user.Id()) {
				if c.authKey() == 0 {
					c.Logger.Errorf("auth.sendCode - email password step has no perm auth key")
					return nil, mtproto.ErrAuthKeyPermEmpty
				}
				if err = c.passwordNeeded(user.Id()); err != nil {
					c.Logger.Errorf("auth.sendCode - email password: %v", err)
					return nil, err
				}
				_, _ = c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
					AuthKeyId: c.MD.PermAuthKeyId,
					UserId:    user.User.Id,
				})
				_ = c.svcCtx.Dao.DelFutureAuthToken(c.ctx, v)
				c.Logger.Infof("auth.sendCode - email future-auth-token needs password: SESSION_PASSWORD_NEEDED")
				return nil, mtproto.ErrSessionPasswordNeeded
			}
			_, _ = c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
				AuthKeyId: c.MD.PermAuthKeyId,
				UserId:    user.User.Id,
			})
			_ = c.svcCtx.Dao.DelFutureAuthToken(c.ctx, v)
			c.pushLoginSecurityNotice(user.Id())
			return mtproto.MakeTLAuthSentCodeSuccess(&mtproto.Auth_SentCode{
				Authorization: mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
					SetupPasswordRequired: false,
					User:                  user.ToSelfUser(),
				}).To_Auth_Authorization(),
			}).To_Auth_SentCode(), nil
		}
	}

	codeData, err2 := c.svcCtx.AuthLogic.DoAuthSendCode(c.ctx,
		authKeyId,
		sessionId,
		email,
		registered,
		false,
		false,
		request.ApiId,
		request.ApiHash,
		func(codeData2 *model.PhoneCodeTransaction) error {
			if codeData2.State == model.CodeStateSent {
				c.Logger.Infof("email code already sent")
				return nil
			}
			if err3 := c.sendEmail(email, codeData2); err3 != nil {
				return err3
			}
			codeData2.PhoneNumberRegistered = registered
			if registered && user != nil {
				c.rememberLoginEmail(user.Id(), email)
				c.rememberLoginCode(user.Id(), codeData2)
				uid := user.Id()
				code := codeData2.PhoneCode
				threading2.WrapperGoFunc(c.ctx, nil, func(ctx context.Context) {
					c.pushSignInMessage(ctx, uid, code)
				})
			}
			return nil
		})
	if err2 != nil {
		c.Logger.Errorf("auth.sendCode outcome email=%s err=%s", verify.MaskEmail(email), rpcErrorCode(err2))
		return nil, err2
	}

	reply = c.finishEmailSentCode(codeData)
	if reply == nil || reply.GetType() == nil {
		c.Logger.Errorf("auth.sendCode outcome email=%s err=empty sent code", verify.MaskEmail(email))
		return nil, mtproto.ErrInternalServerError
	}
	c.Logger.Infof("auth.sendCode outcome email=%s hash=%s type=%s length=%d",
		verify.MaskEmail(email),
		hashPrefix(codeData.PhoneCodeHash),
		reply.GetType().GetPredicateName(),
		reply.GetType().GetLength())
	return reply, nil
}
