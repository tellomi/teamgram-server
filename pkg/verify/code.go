package verify

import "crypto/rand"

// RandomDigits returns a numeric code of exactly n digits.
// A leading zero is kept: the result is a string, never an integer.
// n outside 4..8 becomes 6, the Telegram client default.
func RandomDigits(n int) string {
	n = NormalizeCodeLength(n)
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = '0'
		}
		return string(buf)
	}
	return DigitsFromBytes(buf)
}

// NormalizeCodeLength clamps a configured SMS code length.
// 6 matches current Telegram clients. 4..8 are accepted so an explicit
// TELLOMI_SMS_CODE_LENGTH is not silently rewritten.
func NormalizeCodeLength(n int) int {
	if n < 4 || n > 8 {
		return 6
	}
	return n
}

// DigitsFromBytes maps each byte to '0'..'9', preserving a leading zero.
func DigitsFromBytes(buf []byte) string {
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = '0' + b%10
	}
	return string(out)
}
