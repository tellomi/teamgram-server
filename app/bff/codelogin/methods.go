package codelogin

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/pkg/verify"
)

// RequestPasswordRecovery emails a 2FA recovery code to the account's
// verified recovery address.
// https://core.telegram.org/method/auth.requestPasswordRecovery
func (s *Service) RequestPasswordRecovery(ctx context.Context, userID int64) (*mtproto.Auth_PasswordRecovery, error) {
	if userID == 0 {
		return nil, mtproto.ErrPasswordRecoveryNa
	}
	email, hasPassword, err := password.RecoveryEmail(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !hasPassword {
		email, err = s.LoginEmail(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	if email == "" {
		return nil, mtproto.ErrPasswordRecoveryNa
	}
	code, err := s.freshCode(s.opt.EmailLen)
	if err != nil {
		return nil, err
	}
	sent, err := s.deliverEmail(ctx, email, code)
	if err != nil {
		return nil, err
	}
	if err = s.put(ctx, recoveryKey(userID), record{
		Code:    sent,
		Email:   email,
		UserID:  userID,
		Expires: s.ttl(),
		Purpose: "recovery",
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLAuthPasswordRecovery(&mtproto.Auth_PasswordRecovery{
		EmailPattern: verify.MaskEmail(email),
	}).To_Auth_PasswordRecovery(), nil
}

// CheckRecoveryPassword reports whether the recovery code is valid.
// The code stays valid for auth.recoverPassword.
// https://core.telegram.org/method/auth.checkRecoveryPassword
func (s *Service) CheckRecoveryPassword(ctx context.Context, userID int64, code string) (*mtproto.Bool, error) {
	if _, err := s.touchRecovery(ctx, userID, code, false); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// RecoverPassword consumes the recovery code and logs the session in.
// A malformed new_settings is rejected without consuming the code.
// https://core.telegram.org/method/auth.recoverPassword
func (s *Service) RecoverPassword(ctx context.Context, userID int64, code string, settings *mtproto.Account_PasswordInputSettings) error {
	if settings != nil {
		if err := validateNewSettings(settings); err != nil {
			return err
		}
		if err := password.CheckRecoverySettings(ctx, userID, settings); err != nil {
			return err
		}
	}
	rec, err := s.touchRecovery(ctx, userID, code, true)
	if err != nil {
		return err
	}
	if settings != nil && settings.GetEmail() != nil {
		if err = s.SaveLoginEmail(ctx, userID, settings.GetEmail().GetValue()); err != nil {
			return err
		}
	}
	if err = password.CommitRecovery(ctx, userID, settings); err != nil {
		return err
	}
	_ = rec
	return nil
}

func (s *Service) touchRecovery(ctx context.Context, userID int64, code string, consume bool) (record, error) {
	if strings.TrimSpace(code) == "" {
		return record{}, mtproto.ErrCodeEmpty
	}
	if userID == 0 {
		return record{}, mtproto.ErrPasswordRecoveryExpired
	}
	key := recoveryKey(userID)
	rec, ok, err := s.get(ctx, key)
	if err != nil {
		return record{}, err
	}
	if !ok {
		return record{}, mtproto.ErrPasswordRecoveryExpired
	}
	if rec.Attempts >= s.opt.MaxAttempts {
		return record{}, mtproto.NewErrFloodWaitX(floodWait)
	}
	if !match(rec.Code, strings.TrimSpace(code)) {
		rec.Attempts++
		_ = s.put(ctx, key, rec)
		return record{}, mtproto.ErrCodeInvalid
	}
	if consume {
		_, _ = s.kv.DelCtx(ctx, key)
	}
	return rec, nil
}

// ResetLoginEmail sends a fresh login code to the account's verified email
// through the existing email sender and stores it on the phone-code record
// so auth.signIn accepts it.
// https://core.telegram.org/method/auth.resetLoginEmail
func (s *Service) ResetLoginEmail(ctx context.Context, authKey int64, phone, hash string, userID int64) (*mtproto.Auth_SentCode, error) {
	canon, err := s.canon(phone)
	if err != nil {
		return nil, err
	}
	pc, err := s.phone(ctx, authKey, canon, hash)
	if err != nil {
		if err == mtproto.ErrPhoneNumberInvalid {
			return nil, err
		}
		return nil, mtproto.ErrPhoneCodeExpired
	}
	email, err := s.resetTarget(ctx, authKey, canon, userID, pc.UserID)
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, mtproto.ErrEmailInstallMissing
	}
	key := "tellomi:p0:reset:" + itoa(authKey) + ":" + canon
	if rec, ok, err := s.get(ctx, key); err != nil {
		return nil, err
	} else if ok && rec.Reset {
		return nil, mtproto.ErrTaskAlreadyExists
	}
	code, err := s.freshCode(s.opt.EmailLen)
	if err != nil {
		return nil, err
	}
	sent, err := s.deliverEmail(ctx, email, code)
	if err != nil {
		return nil, err
	}
	pc.Code = sent
	pc.Extra = sent
	pc.SentType = model.SentCodeTypeEmailCode
	pc.EmailPattern = verify.MaskEmail(email)
	pc.Expires = s.ttl()
	if err = s.opt.Phones.Save(ctx, pc); err != nil {
		return nil, err
	}
	if err = s.put(ctx, key, record{Email: email, Reset: true, Expires: s.ttl(), Code: sent}); err != nil {
		return nil, err
	}
	return emailSent(pc.Hash, email, sent), nil
}

func (s *Service) resetTarget(ctx context.Context, authKey int64, canon string, userID, codeUser int64) (string, error) {
	if verify.IsEmail(canon) {
		return canon, nil
	}
	uid := userID
	if uid == 0 {
		uid = codeUser
	}
	if uid == 0 && s.opt.LookupUser != nil {
		id, err := s.opt.LookupUser(ctx, canon)
		if err != nil {
			return "", err
		}
		uid = id
	}
	if uid != 0 {
		email, err := s.LoginEmail(ctx, uid)
		if err != nil {
			return "", err
		}
		if email != "" {
			return email, nil
		}
	}
	pending, err := s.kv.GetCtx(ctx, pendingEmailKey(authKey, canon))
	if err != nil {
		return "", err
	}
	return pending, nil
}

// RequestFirebaseSms does not send Firebase SMS. Mainland China clients
// fall back to auth.resendCode when this returns boolFalse.
// https://core.telegram.org/method/auth.requestFirebaseSms
// https://core.telegram.org/api/auth
func (s *Service) RequestFirebaseSms(ctx context.Context, authKey int64, phone, hash string) (*mtproto.Bool, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, mtproto.ErrPhoneCodeEmpty
	}
	canon, err := s.canon(phone)
	if err != nil {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if _, err = s.phone(ctx, authKey, canon, hash); err != nil {
		// A dead hash should not crash the login screen. resendCode
		// reports PHONE_CODE_EXPIRED. A live hash also returns boolFalse
		// so the client uses next_type instead of waiting on Firebase.
		if err == mtproto.ErrPhoneNumberInvalid {
			return nil, err
		}
	}
	return mtproto.BoolFalse, nil
}

// ReportMissingCode records that the code did not arrive and returns true.
// https://core.telegram.org/method/auth.reportMissingCode
func (s *Service) ReportMissingCode(phone, hash, mnc string, log func(phone, hash, mnc string)) (*mtproto.Bool, error) {
	if _, err := s.canon(phone); err != nil {
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	if log != nil {
		log(phone, hash, mnc)
	}
	return mtproto.BoolTrue, nil
}

// InvalidateSignInCodes drops the listed login codes for the signed-in user.
// Unknown codes are ignored. The method returns true.
// https://core.telegram.org/method/account.invalidateSignInCodes
func (s *Service) InvalidateSignInCodes(ctx context.Context, userID int64, codes []string) (*mtproto.Bool, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	for _, raw := range codes {
		code := stripDashes(raw)
		if code == "" {
			continue
		}
		val, err := s.kv.GetCtx(ctx, signInKey(userID, code))
		if err != nil || val == "" {
			continue
		}
		parts := strings.Split(val, "\t")
		if len(parts) >= 2 && s.opt.Phones != nil {
			authKey := parseInt(parts[0])
			phone := parts[1]
			hash := ""
			if len(parts) >= 3 {
				hash = parts[2]
			}
			_ = s.opt.Phones.Delete(ctx, authKey, phone, hash)
		}
		_, _ = s.kv.DelCtx(ctx, signInKey(userID, code))
	}
	return mtproto.BoolTrue, nil
}

func parseInt(s string) int64 {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

// IssuePhoneConfirm stores the hash a deletion notice would carry.
// account.deleteAccount on Tellomi still deletes immediately, so production
// does not mint these hashes. Tests and a future hold-for-deletion path do.
func (s *Service) IssuePhoneConfirm(ctx context.Context, userID int64, phone string) (string, error) {
	canon, err := s.canon(phone)
	if err != nil {
		return "", err
	}
	hash, err := newHash()
	if err != nil {
		return "", err
	}
	// The deletion link stays valid for the same window Telegram uses
	// before an unconfirmed deletion completes.
	exp := s.now().Add(7 * 24 * time.Hour).Unix()
	if err = s.put(ctx, confirmKey(hash), record{
		Phone:   canon,
		UserID:  userID,
		Hash:    hash,
		Expires: exp,
	}); err != nil {
		return "", err
	}
	return hash, nil
}

// SendConfirmPhoneCode texts the account phone with the login SMS template.
// https://core.telegram.org/method/account.sendConfirmPhoneCode
func (s *Service) SendConfirmPhoneCode(ctx context.Context, hash string) (*mtproto.Auth_SentCode, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, mtproto.ErrHashInvalid
	}
	rec, ok, err := s.get(ctx, confirmKey(hash))
	if err != nil {
		return nil, err
	}
	if !ok || rec.Phone == "" {
		return nil, mtproto.ErrHashInvalid
	}
	code, err := s.freshCode(s.opt.SMSLen)
	if err != nil {
		return nil, err
	}
	sent, err := s.deliverSMS(ctx, rec.Phone, code)
	if err != nil {
		return nil, err
	}
	codeHash, err := newHash()
	if err != nil {
		return nil, err
	}
	if err = s.put(ctx, confirmCodeKey(codeHash), record{
		Code:    sent,
		Phone:   rec.Phone,
		UserID:  rec.UserID,
		Hash:    hash,
		Expires: s.ttl(),
	}); err != nil {
		return nil, err
	}
	return smsSent(codeHash, sent), nil
}

// ConfirmPhone checks the SMS code from SendConfirmPhoneCode.
// https://core.telegram.org/method/account.confirmPhone
func (s *Service) ConfirmPhone(ctx context.Context, phoneCodeHash, phoneCode string) (*mtproto.Bool, error) {
	if strings.TrimSpace(phoneCode) == "" {
		return nil, mtproto.ErrPhoneCodeEmpty
	}
	if strings.TrimSpace(phoneCodeHash) == "" {
		return nil, mtproto.ErrCodeHashInvalid
	}
	key := confirmCodeKey(phoneCodeHash)
	rec, ok, err := s.get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrCodeHashInvalid
	}
	if rec.Attempts >= s.opt.MaxAttempts {
		return nil, mtproto.NewErrFloodWaitX(floodWait)
	}
	if !match(rec.Code, strings.TrimSpace(phoneCode)) {
		rec.Attempts++
		_ = s.put(ctx, key, rec)
		return nil, mtproto.ErrPhoneCodeInvalid
	}
	_, _ = s.kv.DelCtx(ctx, key)
	if rec.Hash != "" {
		_, _ = s.kv.DelCtx(ctx, confirmKey(rec.Hash))
	}
	return mtproto.BoolTrue, nil
}

// Authorization is the auth.authorization body recoverPassword returns.
func Authorization(user *mtproto.User) *mtproto.Auth_Authorization {
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		User: user,
	}).To_Auth_Authorization()
}
