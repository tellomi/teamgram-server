package discoverauth

import (
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

const (
	// EnvEnabled turns credential issuing on. Unset or any other value is off.
	EnvEnabled = "TELLOMI_DISCOVER"
	// EnvSigningKey is standard base64 of the HS256 key. The decoded bytes
	// are the engine's first ENGINE_FEED_AUTH_KEYS entry. At least 32 bytes.
	EnvSigningKey = "TELLOMI_DISCOVER_SIGNING_KEY"
	// EnvPseudonymKey is standard base64 of the HMAC key that derives sub.
	// At least 32 bytes, and different from the signing key. The engine does
	// not have this key. Changing it gives every user a new sub.
	EnvPseudonymKey = "TELLOMI_DISCOVER_PSEUDONYM_KEY"

	minKeyLen = 32
)

var (
	errKey = errors.New("discoverauth: key")
)

// Enabled reports TELLOMI_DISCOVER=1.
func Enabled() bool {
	return os.Getenv(EnvEnabled) == "1"
}

// Keys holds the two raw secrets. Neither value is safe to log.
type Keys struct {
	Signing   []byte
	Pseudonym []byte
}

// LoadKeys reads the two keys from the environment. It returns an error
// when either value is missing, not base64, shorter than 32 bytes, or when
// the two decoded keys are equal. The error text does not include the values.
func LoadKeys() (Keys, error) {
	signing, err := decodeKey(os.Getenv(EnvSigningKey))
	if err != nil {
		return Keys{}, err
	}
	pseudonym, err := decodeKey(os.Getenv(EnvPseudonymKey))
	if err != nil {
		return Keys{}, err
	}
	if hmac.Equal(signing, pseudonym) {
		return Keys{}, errKey
	}
	return Keys{Signing: signing, Pseudonym: pseudonym}, nil
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errKey
	}
	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		buf, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(buf) < minKeyLen {
		return nil, errKey
	}
	return buf, nil
}
