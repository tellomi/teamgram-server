package verify

import "crypto/rand"

// RandomDigits returns a numeric code of length n. n is 5 when it is not 5 or 6.
func RandomDigits(n int) string {
	if n != 5 && n != 6 {
		n = 5
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = '0'
		}
		return string(buf)
	}
	for i := range buf {
		buf[i] = '0' + buf[i]%10
	}
	return string(buf)
}
