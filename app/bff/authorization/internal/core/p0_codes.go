package core

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/app/bff/codelogin"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
)

func (c *AuthorizationCore) userID() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.UserId
}

func (c *AuthorizationCore) authKey() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.PermAuthKeyId
}

// codes is the P0 email, recovery, and login-code service.
// Delivery goes through the same sendEmail / sendSMS helpers as auth.sendCode.
func (c *AuthorizationCore) codes() *codelogin.Service {
	var kv codelogin.KV
	emailLen, smsLen := 6, 6
	ttl := 180 * time.Second
	attempts := 5
	if c != nil && c.svcCtx != nil {
		if c.svcCtx.Dao != nil {
			kv = c.svcCtx.Dao.Cache()
		}
		if n := c.svcCtx.Verify.Email.CodeLength; n > 0 {
			emailLen = n
		}
		if n := c.svcCtx.Verify.SMS.CodeLength; n > 0 {
			smsLen = n
		}
		if sec := c.svcCtx.Verify.EmailLimits.TTLSeconds; sec > 0 {
			ttl = time.Duration(sec) * time.Second
		}
		if n := c.svcCtx.Verify.EmailLimits.MaxAttempts; n > 0 {
			attempts = n
		}
	}
	return codelogin.New(kv, codelogin.Options{
		EmailLen:    emailLen,
		SMSLen:      smsLen,
		TTL:         ttl,
		MaxAttempts: attempts,
		Phones:      authPhoneCodes{c},
		NormalizePhone: func(phone string) (string, error) {
			_, n, err := phonenumber.CheckPhoneNumberInvalid(phone)
			if err != nil {
				return "", mtproto.ErrPhoneNumberInvalid
			}
			return n, nil
		},
		IdentityEmail: password.IdentityEmail,
		IdentityOwner: password.IdentityOwner,
		LookupUser: func(ctx context.Context, phone string) (int64, error) {
			if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
				return 0, nil
			}
			user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUserByPhone(ctx, &userpb.TLUserGetImmutableUserByPhone{Phone: phone})
			if err != nil || user == nil {
				return 0, nil
			}
			return user.Id(), nil
		},
		SendEmail: func(ctx context.Context, to, code string) (string, error) {
			_ = ctx
			tx := &model.PhoneCodeTransaction{PhoneCode: code}
			if err := c.sendEmail(to, tx); err != nil {
				return "", err
			}
			return tx.PhoneCode, nil
		},
		SendSMS: func(ctx context.Context, phone, code string) (string, error) {
			_ = ctx
			tx := &model.PhoneCodeTransaction{PhoneCode: code, PhoneCodeHash: "p0"}
			if err := c.sendSMS(phone, tx); err != nil {
				return "", err
			}
			return tx.PhoneCode, nil
		},
	})
}

type authPhoneCodes struct{ c *AuthorizationCore }

func (a authPhoneCodes) Get(ctx context.Context, authKey int64, phone, hash string) (*codelogin.PhoneCode, error) {
	if a.c == nil || a.c.svcCtx == nil || a.c.svcCtx.Dao == nil {
		return nil, mtproto.ErrPhoneCodeExpired
	}
	data, err := a.c.svcCtx.Dao.GetPhoneCode(ctx, authKey, phone, hash)
	if err != nil || data == nil {
		return nil, err
	}
	return phoneFromTransaction(data), nil
}

func (a authPhoneCodes) Save(ctx context.Context, pc *codelogin.PhoneCode) error {
	if a.c == nil || a.c.svcCtx == nil || a.c.svcCtx.Dao == nil || pc == nil {
		return mtproto.ErrPhoneCodeExpired
	}
	data, err := a.c.svcCtx.Dao.GetPhoneCode(ctx, pc.AuthKeyID, pc.Phone, pc.Hash)
	if err != nil || data == nil {
		return mtproto.ErrPhoneCodeExpired
	}
	data.PhoneCode = pc.Code
	data.PhoneCodeExtraData = pc.Extra
	data.SentCodeType = pc.SentType
	data.EmailPattern = pc.EmailPattern
	if pc.Expires > 0 {
		data.PhoneCodeExpired = int32(pc.Expires)
	}
	data.State = model.CodeStateSent
	return a.c.svcCtx.Dao.UpdatePhoneCodeData(ctx, pc.AuthKeyID, pc.Phone, pc.Hash, data)
}

func (a authPhoneCodes) Delete(ctx context.Context, authKey int64, phone, hash string) error {
	if a.c == nil || a.c.svcCtx == nil || a.c.svcCtx.Dao == nil {
		return nil
	}
	return a.c.svcCtx.Dao.DeletePhoneCode(ctx, authKey, phone, hash)
}

func phoneFromTransaction(data *model.PhoneCodeTransaction) *codelogin.PhoneCode {
	if data == nil {
		return nil
	}
	return &codelogin.PhoneCode{
		AuthKeyID:    data.AuthKeyId,
		Phone:        data.PhoneNumber,
		Hash:         data.PhoneCodeHash,
		Code:         data.PhoneCode,
		Extra:        data.PhoneCodeExtraData,
		Expires:      int64(data.PhoneCodeExpired),
		SentType:     data.SentCodeType,
		EmailPattern: data.EmailPattern,
		Registered:   data.PhoneNumberRegistered,
	}
}

func (c *AuthorizationCore) rememberLoginCode(userID int64, data *model.PhoneCodeTransaction) {
	if c == nil || userID == 0 || data == nil || data.PhoneCode == "" {
		return
	}
	_ = c.codes().RememberSignInCode(c.ctx, userID, data.AuthKeyId, data.PhoneNumber, data.PhoneCode, data.PhoneCodeHash)
}

func (c *AuthorizationCore) rememberLoginEmail(userID int64, email string) {
	if c == nil || userID == 0 || email == "" {
		return
	}
	_ = c.codes().SaveLoginEmail(c.ctx, userID, email)
}
