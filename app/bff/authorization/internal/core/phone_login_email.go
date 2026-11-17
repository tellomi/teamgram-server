package core

import (
	"context"

	"github.com/teamgram/marmota/pkg/threading2"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

// officialPhoneCodeKind is what auth.sendCode returns for a real phone.
// A verified login email is mailed. Otherwise a required-setup policy asks
// the client to collect an address before any SMS. SentCodeTypeNone means
// the existing SMS or in-app code.
func officialPhoneCodeKind(loginEmail string, setupRequired bool) int {
	if loginEmail != "" {
		return model.SentCodeTypeEmailCode
	}
	if setupRequired {
		return model.SentCodeTypeSetUpEmailRequired
	}
	return model.SentCodeTypeNone
}

// loginEmailSetupRequired is TELLOMI_EMAIL_SETUP_REQUIRED, and only when
// the email sender can actually deliver a code. A flag with a disabled
// mailer must not replace SMS.
func (c *AuthorizationCore) loginEmailSetupRequired() bool {
	if c == nil || c.svcCtx == nil {
		return false
	}
	return c.svcCtx.Verify.Email.SetupRequired && c.svcCtx.Verify.Email.Enabled()
}

func (c *AuthorizationCore) lookupLoginEmail(userID int64) (string, error) {
	if c == nil || c.svcCtx == nil || userID == 0 {
		return "", nil
	}
	if c.svcCtx.LoginEmail != nil {
		return c.svcCtx.LoginEmail(c.ctx, userID)
	}
	return c.codes().VerifiedLoginEmail(c.ctx, userID)
}

func (c *AuthorizationCore) verifiedLoginEmailForPhone(phone string) (string, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.UserClient == nil {
		return "", nil
	}
	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUserByPhone(c.ctx, &userpb.TLUserGetImmutableUserByPhone{
		Phone: phone,
	})
	if err != nil || user == nil || user.User == nil || user.Id() == 0 {
		return "", nil
	}
	return c.lookupLoginEmail(user.Id())
}

// officialPhoneLogin applies the official phone-then-email step.
// handled is true when this call owned the code (email sent, or setup
// required and no SMS). The caller then skips the SMS path.
func (c *AuthorizationCore) officialPhoneLogin(registered bool, user *mtproto.ImmutableUser, codeData *model.PhoneCodeTransaction) (bool, error) {
	if codeData == nil {
		return false, nil
	}
	var (
		email string
		err   error
	)
	if registered && user != nil && user.User != nil && user.Id() != 0 {
		email, err = c.lookupLoginEmail(user.Id())
		if err != nil {
			return false, err
		}
	}
	switch officialPhoneCodeKind(email, c.loginEmailSetupRequired()) {
	case model.SentCodeTypeEmailCode:
		if err = c.sendEmail(email, codeData); err != nil {
			return true, err
		}
		codeData.PhoneNumberRegistered = registered
		if registered && user != nil && user.User != nil {
			c.rememberLoginCode(user.Id(), codeData)
			c.pushLoginCode(user.Id(), codeData.PhoneCode)
		}
		return true, nil
	case model.SentCodeTypeSetUpEmailRequired:
		codeData.SentCodeType = model.SentCodeTypeSetUpEmailRequired
		codeData.NextCodeType = model.CodeTypeNone
		codeData.PhoneCode = ""
		codeData.PhoneCodeExtraData = ""
		codeData.EmailPattern = ""
		codeData.State = model.CodeStateSent
		codeData.PhoneNumberRegistered = registered
		return true, nil
	default:
		return false, nil
	}
}

// deliverLoginSMS is the SMS issued after a login email is verified.
// 99966 test numbers keep their fixed code and do not call the provider,
// matching auth.sendCode.
func (c *AuthorizationCore) deliverLoginSMS(phone, code string) (string, error) {
	if fixed, ok := phonenumber.TestPhoneCode(phone); ok {
		return fixed, nil
	}
	tx := &model.PhoneCodeTransaction{PhoneCode: code, PhoneCodeHash: "p0"}
	if err := c.sendSMS(phone, tx); err != nil {
		return "", err
	}
	if tx.PhoneCode != "" {
		return tx.PhoneCode, nil
	}
	return code, nil
}

func (c *AuthorizationCore) pushLoginCode(userID int64, code string) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil || userID == 0 || code == "" {
		return
	}
	threading2.WrapperGoFunc(c.ctx, nil, func(ctx context.Context) {
		c.pushSignInMessage(ctx, userID, code)
	})
}

// attachVerifiedSetupEmail stores the address verified before this phone
// had a user. A missing stash is a normal phone sign-up.
func (c *AuthorizationCore) attachVerifiedSetupEmail(phone string, userID int64) {
	if c == nil || userID == 0 || phone == "" {
		return
	}
	email, err := c.codes().TakeVerifiedSetupEmail(c.ctx, phone)
	if err != nil || email == "" {
		if err != nil {
			c.Logger.Errorf("auth.signUp - setup email: %v", err)
		}
		return
	}
	if err = c.codes().SaveLoginEmail(c.ctx, userID, email); err != nil {
		c.Logger.Errorf("auth.signUp - login email: %v", err)
	}
}
