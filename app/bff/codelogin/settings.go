package codelogin

import (
	"github.com/teamgram/proto/mtproto"
)

func validateNewSettings(settings *mtproto.Account_PasswordInputSettings) error {
	if settings == nil {
		return nil
	}
	algo := settings.GetNewAlgo()
	hash := settings.GetNewPasswordHash()
	email := settings.GetEmail()
	if algo == nil && len(hash) == 0 && email == nil {
		return mtproto.ErrNewSettingsInvalid
	}
	if algo != nil || len(hash) > 0 {
		if !modPowAlgo(algo) || len(algo.GetSalt1()) == 0 || len(algo.GetSalt2()) == 0 || algo.GetG() != 3 || len(algo.GetP()) != 256 {
			return mtproto.ErrNewSaltInvalid
		}
		if len(hash) != 256 {
			return mtproto.ErrNewSettingsInvalid
		}
	}
	if email != nil {
		if _, err := normalizeEmail(email.GetValue()); err != nil {
			return err
		}
	}
	return nil
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
