package core

import (
	"bytes"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"google.golang.org/grpc/status"
)

// Telegram SRP parameters from https://core.telegram.org/api/srp.
// g is 3. p is the 2048-bit modulus the official clients already hard-code.
// salt1 and salt2 are random on every account.getPassword, which is what the
// client appends its own random bytes to when it builds a new verifier.
const (
	srpGenerator         = int32(3)
	secureRandomBytes    = 256
	tmpPasswordBytes     = 32
	tmpPasswordPeriodMin = int32(60)
	tmpPasswordPeriodMax = int32(86400)
	passwordHashLen      = 256
)

// errSRPAInvalid is account.getTmpPassword's documented 400 SRP_A_INVALID.
// https://core.telegram.org/method/account.getTmpPassword
var errSRPAInvalid = status.Error(mtproto.ErrBadRequest, "SRP_A_INVALID")

// srpPrime is passwordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow.p.
var srpPrime = []byte{
	0xc7, 0x1c, 0xae, 0xb9, 0xc6, 0xb1, 0xc9, 0x04, 0x8e, 0x6c, 0x52, 0x2f,
	0x70, 0xf1, 0x3f, 0x73, 0x98, 0x0d, 0x40, 0x23, 0x8e, 0x3e, 0x21, 0xc1,
	0x49, 0x34, 0xd0, 0x37, 0x56, 0x3d, 0x93, 0x0f, 0x48, 0x19, 0x8a, 0x0a,
	0xa7, 0xc1, 0x40, 0x58, 0x22, 0x94, 0x93, 0xd2, 0x25, 0x30, 0xf4, 0xdb,
	0xfa, 0x33, 0x6f, 0x6e, 0x0a, 0xc9, 0x25, 0x13, 0x95, 0x43, 0xae, 0xd4,
	0x4c, 0xce, 0x7c, 0x37, 0x20, 0xfd, 0x51, 0xf6, 0x94, 0x58, 0x70, 0x5a,
	0xc6, 0x8c, 0xd4, 0xfe, 0x6b, 0x6b, 0x13, 0xab, 0xdc, 0x97, 0x46, 0x51,
	0x29, 0x69, 0x32, 0x84, 0x54, 0xf1, 0x8f, 0xaf, 0x8c, 0x59, 0x5f, 0x64,
	0x24, 0x77, 0xfe, 0x96, 0xbb, 0x2a, 0x94, 0x1d, 0x5b, 0xcd, 0x1d, 0x4a,
	0xc8, 0xcc, 0x49, 0x88, 0x07, 0x08, 0xfa, 0x9b, 0x37, 0x8e, 0x3c, 0x4f,
	0x3a, 0x90, 0x60, 0xbe, 0xe6, 0x7c, 0xf9, 0xa4, 0xa4, 0xa6, 0x95, 0x81,
	0x10, 0x51, 0x90, 0x7e, 0x16, 0x27, 0x53, 0xb5, 0x6b, 0x0f, 0x6b, 0x41,
	0x0d, 0xba, 0x74, 0xd8, 0xa8, 0x4b, 0x2a, 0x14, 0xb3, 0x14, 0x4e, 0x0e,
	0xf1, 0x28, 0x47, 0x54, 0xfd, 0x17, 0xed, 0x95, 0x0d, 0x59, 0x65, 0xb4,
	0xb9, 0xdd, 0x46, 0x58, 0x2d, 0xb1, 0x17, 0x8d, 0x16, 0x9c, 0x6b, 0xc4,
	0x65, 0xb0, 0xd6, 0xff, 0x9c, 0xa3, 0x92, 0x8f, 0xef, 0x5b, 0x9a, 0xe4,
	0xe4, 0x18, 0xfc, 0x15, 0xe8, 0x3e, 0xbe, 0xa0, 0xf8, 0x7f, 0xa9, 0xff,
	0x5e, 0xed, 0x70, 0x05, 0x0d, 0xed, 0x28, 0x49, 0xf4, 0x7b, 0xf9, 0x59,
	0xd9, 0x56, 0x85, 0x0c, 0xe9, 0x29, 0x85, 0x1f, 0x0d, 0x81, 0x15, 0xf6,
	0x35, 0xb1, 0x05, 0xee, 0x2e, 0x4e, 0x15, 0xd0, 0x4b, 0x24, 0x54, 0xbf,
	0x6f, 0x4f, 0xad, 0xf0, 0x34, 0xb1, 0x04, 0x03, 0x11, 0x9c, 0xd8, 0xe3,
	0xb9, 0x2f, 0xcc, 0x5b,
}

// accountPassword is the documented state when no cloud password, recovery
// email, or reset is configured.
// https://core.telegram.org/method/account.getPassword
// https://core.telegram.org/constructor/account.password
// https://core.telegram.org/api/srp
func (c *TwoFaCore) accountPassword() *mtproto.Account_Password {
	newAlgo := mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
		Salt1: crypto.RandomBytes(8),
		Salt2: crypto.RandomBytes(16),
		G:     srpGenerator,
		P:     srpPrime,
	}).To_PasswordKdfAlgo()
	newSecure := mtproto.MakeTLSecurePasswordKdfAlgoPBKDF2(&mtproto.SecurePasswordKdfAlgo{
		Salt: crypto.RandomBytes(8),
	}).To_SecurePasswordKdfAlgo()
	return mtproto.MakeTLAccountPassword(&mtproto.Account_Password{
		NewAlgo:       newAlgo,
		NewSecureAlgo: newSecure,
		SecureRandom:  crypto.RandomBytes(secureRandomBytes),
	}).To_Account_Password()
}

func emptyPasswordSettings() *mtproto.Account_PasswordSettings {
	return mtproto.MakeTLAccountPasswordSettings(&mtproto.Account_PasswordSettings{}).To_Account_PasswordSettings()
}

// checkEmptyPassword accepts inputCheckPasswordEmpty, which is what clients
// send when account.getPassword did not set has_password.
// A submitted SRP proof has no srp_id to match, so it is SRP_ID_INVALID.
func (c *TwoFaCore) checkEmptyPassword(password *mtproto.InputCheckPasswordSRP) error {
	if c.accountPassword().GetHasPassword() {
		return mtproto.ErrPasswordHashInvalid
	}
	switch passwordPredicate(password) {
	case mtproto.Predicate_inputCheckPasswordEmpty:
		return nil
	case mtproto.Predicate_inputCheckPasswordSRP:
		return mtproto.ErrSrpIdInvalid
	default:
		return mtproto.ErrPasswordHashInvalid
	}
}

// checkTmpPassword uses the errors documented for account.getTmpPassword.
// https://core.telegram.org/method/account.getTmpPassword
func (c *TwoFaCore) checkTmpPassword(password *mtproto.InputCheckPasswordSRP) error {
	switch passwordPredicate(password) {
	case mtproto.Predicate_inputCheckPasswordEmpty:
		return nil
	case mtproto.Predicate_inputCheckPasswordSRP:
		if len(password.GetA()) == 0 {
			return errSRPAInvalid
		}
		return mtproto.ErrPasswordHashInvalid
	default:
		return mtproto.ErrPasswordHashInvalid
	}
}

func passwordPredicate(password *mtproto.InputCheckPasswordSRP) string {
	if password == nil {
		return ""
	}
	if password.PredicateName != "" {
		return password.PredicateName
	}
	switch password.Constructor {
	case mtproto.CRC32_inputCheckPasswordEmpty:
		return mtproto.Predicate_inputCheckPasswordEmpty
	case mtproto.CRC32_inputCheckPasswordSRP:
		return mtproto.Predicate_inputCheckPasswordSRP
	default:
		return ""
	}
}

// validateNewSettings applies the errors from account.updatePasswordSettings
// for an account that has no cloud password yet.
// https://core.telegram.org/method/account.updatePasswordSettings
func validateNewSettings(settings *mtproto.Account_PasswordInputSettings) error {
	if settings == nil || (settings.GetNewAlgo() == nil && len(settings.GetNewPasswordHash()) == 0) {
		return mtproto.ErrNewSettingsEmpty
	}
	algo := settings.GetNewAlgo()
	if !modPowAlgo(algo) || len(algo.GetSalt1()) == 0 || len(algo.GetSalt2()) == 0 || algo.GetG() != srpGenerator || !bytes.Equal(algo.GetP(), srpPrime) {
		return mtproto.ErrNewSaltInvalid
	}
	if len(settings.GetNewPasswordHash()) != passwordHashLen {
		return mtproto.ErrNewSettingsInvalid
	}
	if email := settings.GetEmail(); email != nil && !validRecoveryEmail(email.GetValue()) {
		return mtproto.ErrEmailInvalid
	}
	// A verifier is not stored. Returning success would make the client treat
	// 2FA as enabled while account.getPassword still has has_password unset.
	return mtproto.ErrNewSettingsInvalid
}

func modPowAlgo(algo *mtproto.PasswordKdfAlgo) bool {
	if algo == nil {
		return false
	}
	if algo.PredicateName == mtproto.Predicate_passwordKdfAlgoModPow {
		return true
	}
	return algo.Constructor == mtproto.CRC32_passwordKdfAlgoModPow
}

func validRecoveryEmail(email string) bool {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 128 || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at != strings.LastIndexByte(email, '@') || at >= len(email)-1 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// clampTmpPeriod keeps period inside the documented 60..86400 second window.
// https://core.telegram.org/method/account.getTmpPassword
func clampTmpPeriod(period int32) int32 {
	if period < tmpPasswordPeriodMin {
		return tmpPasswordPeriodMin
	}
	if period > tmpPasswordPeriodMax {
		return tmpPasswordPeriodMax
	}
	return period
}

func (c *TwoFaCore) makeTmpPassword(period int32) *mtproto.Account_TmpPassword {
	return mtproto.MakeTLAccountTmpPassword(&mtproto.Account_TmpPassword{
		TmpPassword: crypto.RandomBytes(tmpPasswordBytes),
		ValidUntil:  int32(time.Now().Unix()) + period,
	}).To_Account_TmpPassword()
}
