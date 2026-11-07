package verify

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
)

// PhoneQuotaError maps a limiter failure onto the login-screen error.
// A daily cap is PHONE_NUMBER_FLOOD: the Android and iOS sendCode handlers
// show the dedicated "too many codes" alert for that text. FLOOD_WAIT on
// that screen is a generic flood string and does not show the wait.
// A resend cooldown stays FLOOD_WAIT_<seconds>.
func PhoneQuotaError(err error) error {
	return quotaError(err, true)
}

// EmailQuotaError is the same mapping. Email login uses auth.sendCode, so
// the client shows the same PHONE_NUMBER_FLOOD alert.
func EmailQuotaError(err error) error {
	return quotaError(err, false)
}

func quotaError(err error, phone bool) error {
	var lim *LimitError
	if !errors.As(err, &lim) {
		return err
	}
	if lim.Daily {
		_ = phone
		return mtproto.ErrPhoneNumberFlood
	}
	retry := lim.Retry
	if retry <= 0 {
		retry = 60
	}
	return mtproto.NewErrFloodWaitX(int32(retry))
}
