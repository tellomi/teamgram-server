package relay

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"
)

// TURNPassword is the coturn REST (use-auth-secret) password.
// username is "<unix-expiry>:<name>" and password is base64(HMAC-SHA1(secret, username)).
func TURNPassword(secret, username string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// TURNUsername builds a username that coturn accepts until ttl elapses.
func TURNUsername(name string, ttl time.Duration, now time.Time) string {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	exp := now.Add(ttl).Unix()
	return fmt.Sprintf("%d:%s", exp, name)
}
