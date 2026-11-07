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

// RequireCountry rejects a number outside the allow list before any SMS send.
// allowInternational accepts any non-empty number. An empty list means +86 only.
func RequireCountry(digits string, allowInternational bool, countries []string) error {
	digits = strings.TrimPrefix(strings.TrimSpace(digits), "+")
	if digits == "" {
		return ErrInternationalDisabled
	}
	if allowInternational {
		return nil
	}
	if len(countries) == 0 {
		countries = []string{"86"}
	}
	for _, code := range countries {
		code = strings.TrimPrefix(strings.TrimSpace(code), "+")
		if code == "" {
			continue
		}
		if code == "86" {
			if IsChina(digits) {
				return nil
			}
			continue
		}
		if strings.HasPrefix(digits, code) {
			return nil
		}
	}
	return ErrInternationalDisabled
}
