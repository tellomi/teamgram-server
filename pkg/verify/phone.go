package verify

import (
	"errors"
	"strings"
)

var ErrInternationalDisabled = errors.New("international sms is disabled")

// IsChina reports whether digits are an E.164 mainland China mobile number
// without the leading plus (86 + 11-digit mobile).
func IsChina(digits string) bool {
	return len(digits) == 13 && strings.HasPrefix(digits, "86") && digits[2] == '1'
}

// FormatAliyunPhone returns the PhoneNumbers value Aliyun SendSms expects.
// Mainland numbers are the 11-digit national form. Other countries are sent
// as country-code plus national number, and only when allowInternational is set.
func FormatAliyunPhone(digits string, allowInternational bool) (string, error) {
	digits = strings.TrimPrefix(strings.TrimSpace(digits), "+")
	if IsChina(digits) {
		return digits[2:], nil
	}
	if !allowInternational || digits == "" {
		return "", ErrInternationalDisabled
	}
	return digits, nil
}
