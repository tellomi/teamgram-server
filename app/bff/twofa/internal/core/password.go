package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/pkg/srp"
	"google.golang.org/grpc/status"
)

const (
	srpGenerator         = srp.Generator
	secureRandomBytes    = 256
	tmpPasswordBytes     = 32
	tmpPasswordPeriodMin = int32(60)
	tmpPasswordPeriodMax = int32(86400)
	passwordHashLen      = 256
)

// errSRPAInvalid is account.getTmpPassword's documented 400 SRP_A_INVALID.
// https://core.telegram.org/method/account.getTmpPassword
var errSRPAInvalid = status.Error(mtproto.ErrBadRequest, "SRP_A_INVALID")

// accountPassword builds the no-password account.password shape.
// Handlers with a user id go through the password service instead.
func (c *TwoFaCore) accountPassword() *mtproto.Account_Password {
	newAlgo := mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
		Salt1: crypto.RandomBytes(8),
		Salt2: crypto.RandomBytes(16),
		G:     srpGenerator,
		P:     srp.Prime,
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

// checkTmpPassword uses the errors documented for account.getTmpPassword
// when the account has no cloud password.
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
